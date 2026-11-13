package service

import (
	"chatnip/internal/kit/config"
	"chatnip/internal/kit/media"
	hsemDomain "chatnip/internal/modules/hsem/domain"
	hsemService "chatnip/internal/modules/hsem/service"
	"chatnip/internal/modules/whatsapp/domain"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rs/zerolog"
	"golang.org/x/net/proxy"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type qrSessionState struct {
	status string
	qr     string
	cancel context.CancelFunc
}

type WhatsAppService struct {
	repo         domain.WhatsAppRepository
	clientRepo   domain.ClientRepository
	dispatcher   *EventDispatcher
	cfg          *config.Config
	logger       zerolog.Logger
	hsemEngine   *hsemService.HSEMEngine
	hsemExecutor *HSEMExecutor
	publisher    domain.EventPublisher
	downloader   domain.FileDownloader
	qrStates     sync.Map
	serializer   *SessionSerializer
}

func NewWhatsAppService(
	repo domain.WhatsAppRepository,
	clientRepo domain.ClientRepository,
	dispatcher *EventDispatcher,
	cfg *config.Config,
	logger zerolog.Logger,
	publisher domain.EventPublisher,
	downloader domain.FileDownloader,
) *WhatsAppService {
	mathUtils := hsemService.NewMathUtils()
	svc := &WhatsAppService{
		repo:         repo,
		clientRepo:   clientRepo,
		dispatcher:   dispatcher,
		cfg:          cfg,
		logger:       logger,
		hsemEngine:   hsemService.NewHSEMEngine(mathUtils),
		hsemExecutor: NewHSEMExecutor(logger),
		publisher:    publisher,
		downloader:   downloader,
	}
	svc.serializer = NewSessionSerializer(svc)
	return svc
}

func (s *WhatsAppService) ProcessBackgroundMessage(ctx context.Context, payload domain.SendMessagePayload, ack domain.Acknowledger) error {
	payload.RemoteJID = s.formatJID(payload.RemoteJID, payload.IsGroup)
	if _, ok := s.clientRepo.GetClient(payload.SessionID); !ok {

		session, err := s.repo.GetMapping(ctx, payload.SessionID)

		if err == nil && session != nil {

			return domain.ErrSessionLoading
		}

		return domain.ErrSessionNotFound
	}

	var mediaData []byte
	if payload.MediaURL != "" {
		var err error
		mediaData, err = s.downloader.Download(payload.MediaURL)
		if err != nil {
			return fmt.Errorf("failed to download media: %w", err)
		}

		if payload.MediaType == "audio" || payload.IsPTT {

			analysisCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			metadata, err := media.AnalyzeAudio(analysisCtx, mediaData)
			cancel()

			if err != nil {
				s.logger.Warn().Err(err).Msg("Failed to analyze audio metadata, using fallbacks")

			} else {

				payload.Duration = metadata.DurationSeconds

				if payload.IsPTT {
					payload.Waveform = metadata.Waveform
				}
			}

			s.logger.Debug().
				Uint32("duration", payload.Duration).
				Int("waveform_len", len(payload.Waveform)).
				Msg("Audio metadata calculated")
		}

		mimeType := http.DetectContentType(mediaData)
		if payload.MediaType == "image" && mimeType == "image/webp" {
			converted, err := media.ConvertWebPToJPEG(mediaData, 80)
			if err != nil {
				s.logger.Error().Err(err).Msg("failed to convert webp to jpeg")
			} else {
				mediaData = converted
			}
		}
	}

	return s.serializer.Enqueue(ctx, payload, mediaData, ack)
}

func (s *WhatsAppService) ExecuteBurstStep(ctx context.Context, payload domain.SendMessagePayload, mediaData []byte, isBurstContinuing bool) (string, error) {
	client, ok := s.clientRepo.GetClient(payload.SessionID)
	if !ok {
		return "", domain.ErrSessionNotFound
	}
	if !client.IsLoggedIn() {
		return "", errors.New("session not logged in")
	}

	targetPersona := hsemDomain.PersonaStandard
	if payload.IsPriority {
		targetPersona = hsemDomain.PersonaRushed
	}

	input := hsemDomain.SimulationInput{
		Persona:         targetPersona,
		InputTextLength: len(payload.Text),
		OutputText:      payload.Text,
		CurrentTime:     time.Now(),
		CurrentFatigue:  0.0,
		MessageType:     hsemDomain.MsgTypeText,
	}

	if len(mediaData) > 0 {
		input.FileSizeKB = len(mediaData) / 1024

		switch payload.MediaType {
		case "image":
			input.MessageType = hsemDomain.MsgTypeImage
		case "video":
			input.MessageType = hsemDomain.MsgTypeVideo
		case "audio":
			if payload.IsPTT {
				input.MessageType = hsemDomain.MsgTypeAudioPTT
				input.MediaDuration = 5 * time.Second
			} else {
				input.MessageType = hsemDomain.MsgTypeAudioPTT
			}
		case "document":
			input.MessageType = hsemDomain.MsgTypeDocument
		case "sticker":
			input.MessageType = hsemDomain.MsgTypeSticker
		}
	}

	if payload.Action == "reaction" || payload.Action == "revoke" || payload.Action == "read" || payload.Action == "mark_read" || payload.IsBroadcastOrChannelTarget() {
		quickPlan := hsemDomain.FingerprintPlan{
			TotalDuration: 300 * time.Millisecond,
			Steps: []hsemDomain.ActionStep{
				{
					Type:      hsemDomain.StepAction,
					Timestamp: 200 * time.Millisecond,
					Metadata:  "send_message",
				},
			},
		}
		return s.hsemExecutor.Execute(ctx, client, quickPlan, payload, mediaData)
	}

	plan := s.hsemEngine.GenerateFingerprint(input)

	if isBurstContinuing {
		var optimizedSteps []hsemDomain.ActionStep
		for _, step := range plan.Steps {
			if step.Metadata == "paused" {
				continue
			}
			optimizedSteps = append(optimizedSteps, step)
		}
		plan.Steps = optimizedSteps
	}

	return s.hsemExecutor.Execute(ctx, client, plan, payload, mediaData)
}

func (s *WhatsAppService) QueueMessage(ctx context.Context, payload domain.SendMessagePayload) error {
	routingKey := "msg.send.normal"
	if payload.IsPriority {
		routingKey = "msg.send.priority"
	}

	return s.publisher.Publish(ctx, routingKey, payload)
}

func (s *WhatsAppService) RestoreSessions() {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	mappings, err := s.repo.GetSessions(ctx, "")
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to load sessions")
		return
	}

	s.logger.Info().Int("count", len(mappings)).Msg("Starting session restoration...")

	concurrencyLimit := 10
	sem := make(chan struct{}, concurrencyLimit)
	var wg sync.WaitGroup

	for _, session := range mappings {
		wg.Add(1)

		sem <- struct{}{}

		go func(sess *domain.Session) {
			defer wg.Done()
			defer func() { <-sem }()

			time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)

			client, err := s.clientRepo.CreateClient(ctx, *sess)
			if err != nil {
				s.logger.Error().Err(err).Str("session_id", sess.ID).Msg("Failed to restore client")
				return
			}

			s.registerHandlers(client, sess.ID, sess.TenantID)

			if err := client.Connect(); err != nil {
				s.logger.Debug().Str("session_id", sess.ID).Msg("Background restore connection failed (will retry automatically)")
			}
		}(session)
	}

	wg.Wait()
	s.logger.Info().Msg("Session restoration process finished")
}

func (s *WhatsAppService) registerHandlers(client domain.WhatsAppClient, sessionID, tenantID string) {
	client.RegisterHandler(func(rawEvt interface{}) {
		ctx := context.Background()

		if evt, ok := rawEvt.(domain.PairSuccessEvent); ok {
			s.clearQRState(sessionID)
			s.dispatcher.Dispatch(ctx, evt, sessionID, tenantID)
			return
		}

		if evt, ok := rawEvt.(domain.LoggedOutEvent); ok {
			s.handleLoggedOut(sessionID, evt)
			return
		}

		if evt, ok := rawEvt.(domain.Event); ok {
			s.dispatcher.Dispatch(ctx, evt, sessionID, tenantID)
		}
	})
}

func (s *WhatsAppService) Login(ctx context.Context, sessionID, tenantID, requestProxyURL string) error {
	finalProxyURL := strings.TrimSpace(requestProxyURL)
	if finalProxyURL == "" {
		return domain.ErrProxyRequired
	}

	if err := s.cleanupExistingSession(sessionID); err != nil {
		return err
	}

	session := domain.Session{
		ID:       sessionID,
		TenantID: tenantID,
		ProxyURL: finalProxyURL,
	}

	client, err := s.clientRepo.CreateClient(ctx, session)
	if err != nil {
		return err
	}

	s.registerHandlers(client, sessionID, tenantID)

	err = s.repo.SaveMapping(ctx, &session)
	if err != nil {
		s.cleanupClient(sessionID)
		return fmt.Errorf("failed to persist session info: %w", err)
	}

	qrCtx, qrCancel := context.WithCancel(context.Background())
	s.setQRState(sessionID, qrSessionState{
		status: "PENDING_QR",
		cancel: qrCancel,
	})

	qrChan, err := client.GetQRChannel(qrCtx)
	if err != nil {
		s.clearQRState(sessionID)
		s.cleanupClient(sessionID)
		_ = s.repo.DeleteMapping(ctx, sessionID)
		return err
	}

	if err := client.Connect(); err != nil {
		s.clearQRState(sessionID)
		s.cleanupClient(sessionID)
		_ = s.repo.DeleteMapping(ctx, sessionID)
		return fmt.Errorf("socket connection failed: %w", err)
	}

	go s.listenQRChannel(qrCtx, qrChan, sessionID, client, qrCancel)
	return nil
}

func (s *WhatsAppService) LoginWithPairCode(ctx context.Context, sessionID, tenantID, phone, requestProxyURL string) (string, error) {
	finalProxyURL := strings.TrimSpace(requestProxyURL)
	if finalProxyURL == "" {
		return "", domain.ErrProxyRequired
	}

	cleanPhone := strings.TrimSpace(strings.ReplaceAll(phone, "+", ""))
	if cleanPhone == "" {
		return "", errors.New("phone is required for code pairing")
	}

	if err := s.cleanupExistingSession(sessionID); err != nil {
		return "", err
	}

	session := domain.Session{
		ID:       sessionID,
		TenantID: tenantID,
		ProxyURL: finalProxyURL,
	}

	client, err := s.clientRepo.CreateClient(ctx, session)
	if err != nil {
		return "", err
	}

	s.registerHandlers(client, sessionID, tenantID)

	err = s.repo.SaveMapping(ctx, &session)
	if err != nil {
		s.cleanupClient(sessionID)
		return "", fmt.Errorf("failed to persist session info: %w", err)
	}

	if err := client.Connect(); err != nil {
		s.cleanupClient(sessionID)
		_ = s.repo.DeleteMapping(ctx, sessionID)
		return "", fmt.Errorf("socket connection failed: %w", err)
	}

	time.Sleep(1 * time.Second)

	code, err := client.PairPhone(ctx, cleanPhone)
	if err != nil {
		s.cleanupClient(sessionID)
		_ = s.repo.DeleteMapping(ctx, sessionID)
		return "", fmt.Errorf("pair phone failed: %w", err)
	}

	return code, nil
}

func (s *WhatsAppService) cleanupExistingSession(sessionID string) error {
	if existingClient, exists := s.clientRepo.GetClient(sessionID); exists {
		if existingClient.IsLoggedIn() && existingClient.IsConnected() {
			return domain.ErrSessionAlreadyActive
		}
		s.clearQRState(sessionID)
		s.clientRepo.RemoveClient(sessionID)
	}
	return nil
}

func (s *WhatsAppService) cleanupClient(sessionID string) {
	s.clientRepo.RemoveClient(sessionID)
}

func (s *WhatsAppService) setQRState(sessionID string, state qrSessionState) {
	s.qrStates.Store(sessionID, state)
}

func (s *WhatsAppService) getQRState(sessionID string) (qrSessionState, bool) {
	val, ok := s.qrStates.Load(sessionID)
	if !ok {
		return qrSessionState{}, false
	}
	state, ok := val.(qrSessionState)
	return state, ok
}

func (s *WhatsAppService) clearQRState(sessionID string) {
	if val, loaded := s.qrStates.LoadAndDelete(sessionID); loaded {
		if state, ok := val.(qrSessionState); ok && state.cancel != nil {
			state.cancel()
		}
	}
}

func (s *WhatsAppService) listenQRChannel(qrCtx context.Context, qrChan <-chan string, sessionID string, client domain.WhatsAppClient, qrCancel context.CancelFunc) {
	defer qrCancel()
	timeout := time.NewTimer(3 * time.Minute)
	defer timeout.Stop()

	for {
		select {
		case <-qrCtx.Done():
			return
		case code, ok := <-qrChan:
			if !ok {
				if client != nil && client.IsLoggedIn() {
					s.clearQRState(sessionID)
				} else if qrCtx.Err() == nil {
					s.setQRState(sessionID, qrSessionState{status: "EXPIRED"})
				}
				return
			}
			s.setQRState(sessionID, qrSessionState{
				status: "QR_READY",
				qr:     domain.ToDeepLink(code),
				cancel: qrCancel,
			})
		case <-timeout.C:
			if client != nil && !client.IsLoggedIn() {
				s.cleanupClient(sessionID)
				s.setQRState(sessionID, qrSessionState{status: "EXPIRED"})
			} else {
				s.clearQRState(sessionID)
			}
			return
		}
	}
}

func (s *WhatsAppService) buildSessionStatus(sess *domain.Session) *domain.SessionStatus {
	status := &domain.SessionStatus{
		ID:        sess.ID,
		TenantID:  sess.TenantID,
		ProxyURL:  sess.ProxyURL,
		CreatedAt: sess.CreatedAt,
		Status:    "DISCONNECTED",
	}

	if client, ok := s.clientRepo.GetClient(sess.ID); ok {
		status.Connected = client.IsConnected()
		status.LoggedIn = client.IsLoggedIn()
		if status.Connected {
			status.Status = "CONNECTED"
		}
		if status.LoggedIn {
			status.JID = client.GetJID()
			status.Status = "LOGGED_IN"
			return status
		}
	}

	if qrState, ok := s.getQRState(sess.ID); ok {
		status.Status = qrState.status
		status.QR = qrState.qr
	}

	return status
}

func (s *WhatsAppService) ListSessions(ctx context.Context, tenantID string) ([]*domain.SessionStatus, error) {
	sessions, err := s.repo.GetSessions(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	var results []*domain.SessionStatus
	for _, sess := range sessions {
		results = append(results, s.buildSessionStatus(sess))
	}
	return results, nil
}

func (s *WhatsAppService) GetStatus(sessionID string) (*domain.SessionStatus, error) {
	session, err := s.repo.GetMapping(context.Background(), sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return &domain.SessionStatus{Status: "NOT_FOUND"}, nil
	}

	return s.buildSessionStatus(session), nil
}

func (s *WhatsAppService) Logout(ctx context.Context, sessionID string) error {
	s.clearQRState(sessionID)

	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok {
		return s.repo.DeleteMapping(ctx, sessionID)
	}

	if client.IsLoggedIn() {
		_ = client.Logout(context.Background())
	}

	s.clientRepo.RemoveClient(sessionID)
	return s.repo.DeleteMapping(ctx, sessionID)
}

func (s *WhatsAppService) handleLoggedOut(sessionID string, evt domain.LoggedOutEvent) {
	s.clearQRState(sessionID)
	s.clientRepo.RemoveClient(sessionID)
	_ = s.repo.DeleteMapping(context.Background(), sessionID)
}

func (s *WhatsAppService) Shutdown() {
	s.clientRepo.DisconnectAll()
}

func (s *WhatsAppService) SendMediaMessage(ctx context.Context, sessionID string, req domain.MediaMessageInput, mediaURL, remoteJID string) (string, error) {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return "", errors.New("session not connected")
	}

	data, err := s.downloader.Download(mediaURL)
	if err != nil {
		return "", fmt.Errorf("failed to download media: %w", err)
	}
	req.MediaData = data

	return client.SendMedia(ctx, remoteJID, req)
}

func (s *WhatsAppService) UpdateProxy(ctx context.Context, sessionID, newProxyURL string) error {
	newProxyURL = strings.TrimSpace(newProxyURL)
	if newProxyURL == "" {
		return domain.ErrProxyRequired
	}

	session, err := s.repo.GetMapping(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return domain.ErrSessionNotFound
	}

	wasConnected := false
	if client, exists := s.clientRepo.GetClient(sessionID); exists {
		wasConnected = client.IsConnected()
		s.clientRepo.RemoveClient(sessionID)
	}

	err = s.repo.UpdateProxy(ctx, sessionID, newProxyURL)
	if err != nil {
		return err
	}

	session.ProxyURL = newProxyURL

	client, err := s.clientRepo.CreateClient(ctx, *session)
	if err != nil {
		return fmt.Errorf("failed to recreate client with new proxy: %w", err)
	}

	s.registerHandlers(client, sessionID, session.TenantID)

	if wasConnected {
		go func() {
			_ = client.Connect()
		}()
	}

	return nil
}

func (s *WhatsAppService) ValidateProxy(ctx context.Context, rawProxyURL string) (*domain.ProxyValidationResult, error) {
	proxyURL := strings.TrimSpace(rawProxyURL)
	if proxyURL == "" {
		return nil, domain.ErrProxyRequired
	}

	transport, err := buildProxyTransport(proxyURL)
	if err != nil {
		return &domain.ProxyValidationResult{
			Valid: false,
			Error: err.Error(),
		}, nil
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   8 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.ipify.org?format=json", nil)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	resp, err := httpClient.Do(req)
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		return &domain.ProxyValidationResult{
			Valid:     false,
			LatencyMs: latencyMs,
			Error:     err.Error(),
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusProxyAuthRequired {
		return &domain.ProxyValidationResult{
			Valid:     false,
			LatencyMs: latencyMs,
			Error:     "proxy authentication failed (407)",
		}, nil
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return &domain.ProxyValidationResult{
			Valid:     false,
			LatencyMs: latencyMs,
			Error:     fmt.Sprintf("proxy check returned status %d", resp.StatusCode),
		}, nil
	}

	var ipResp struct {
		IP string `json:"ip"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&ipResp)

	return &domain.ProxyValidationResult{
		Valid:     true,
		ExitIP:    ipResp.IP,
		LatencyMs: latencyMs,
	}, nil
}

func buildProxyTransport(rawProxyURL string) (*http.Transport, error) {
	parsed, err := url.Parse(rawProxyURL)
	if err != nil || parsed.Host == "" {
		return nil, errors.New("invalid proxy_url format")
	}

	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return &http.Transport{
			Proxy: http.ProxyURL(parsed),
		}, nil
	case "socks5", "socks5h":
		dialer, err := proxy.FromURL(parsed, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("invalid socks5 proxy: %w", err)
		}
		return &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				if cd, ok := dialer.(proxy.ContextDialer); ok {
					return cd.DialContext(ctx, network, addr)
				}
				return dialer.Dial(network, addr)
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s (expected http, https, socks5 or socks5h)", parsed.Scheme)
	}
}

func (s *WhatsAppService) CreateGroup(ctx context.Context, sessionID, groupName string, participants []string) (string, error) {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return "", errors.New("session not connected")
	}

	groupID, err := client.CreateGroup(ctx, groupName, participants)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to create group")
		return "", err
	}
	s.logger.Info().Str("group_id", groupID).Msg("Group created, applying HSEM settings...")

	delay1 := s.hsemEngine.CalculateInteractionDelay()
	time.Sleep(delay1)

	if err := client.SetGroupLocked(ctx, groupID, true); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to lock group info")
	} else {
		s.logger.Debug().Msg("Group info locked (Admins only)")
	}

	delay2 := s.hsemEngine.CalculateInteractionDelay()
	time.Sleep(delay2)

	if err := client.SetGroupAnnounce(ctx, groupID, true); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to set group announce mode")
	} else {
		s.logger.Debug().Msg("Group announce mode set (Admins only)")
	}

	return groupID, nil
}

func (s *WhatsAppService) ListGroups(ctx context.Context, sessionID string) ([]domain.Group, error) {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return nil, errors.New("session not connected")
	}

	return client.ListGroups(ctx)
}

func (s *WhatsAppService) SetGroupMessageSetting(ctx context.Context, sessionID, groupJID string, onlyAdmins bool) error {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return errors.New("session not connected")
	}

	return client.SetGroupAnnounce(ctx, groupJID, onlyAdmins)
}

func (s *WhatsAppService) GetGroupInfo(ctx context.Context, sessionID, groupJID string) (*domain.GroupMetadata, error) {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return nil, errors.New("session not connected")
	}
	return client.GetGroupInfo(ctx, groupJID)
}

func (s *WhatsAppService) SetGroupName(ctx context.Context, sessionID, groupJID, name string) error {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return errors.New("session not connected")
	}
	return client.SetGroupName(ctx, groupJID, name)
}

func (s *WhatsAppService) SetGroupTopic(ctx context.Context, sessionID, groupJID, topic string) error {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return errors.New("session not connected")
	}
	return client.SetGroupTopic(ctx, groupJID, topic)
}

func (s *WhatsAppService) SetGroupPhoto(ctx context.Context, sessionID, groupJID string, avatar []byte) (string, error) {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return "", errors.New("session not connected")
	}
	return client.SetGroupPhoto(ctx, groupJID, avatar)
}

func (s *WhatsAppService) SetGroupInfoLocked(ctx context.Context, sessionID, groupJID string, locked bool) error {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return errors.New("session not connected")
	}
	return client.SetGroupLocked(ctx, groupJID, locked)
}

func (s *WhatsAppService) UpdateGroupParticipants(ctx context.Context, sessionID, groupJID string, participants []string, action string) ([]domain.GroupParticipant, error) {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return nil, errors.New("session not connected")
	}
	return client.UpdateGroupParticipants(ctx, groupJID, participants, action)
}

func (s *WhatsAppService) LeaveGroup(ctx context.Context, sessionID, groupJID string) error {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return errors.New("session not connected")
	}
	return client.LeaveGroup(ctx, groupJID)
}

func (s *WhatsAppService) GetGroupInviteLink(ctx context.Context, sessionID, groupJID string, reset bool) (string, error) {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return "", errors.New("session not connected")
	}
	return client.GetGroupInviteLink(ctx, groupJID, reset)
}

func (s *WhatsAppService) GetGroupInfoFromLink(ctx context.Context, sessionID, code string) (*domain.GroupInviteInfo, error) {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return nil, errors.New("session not connected")
	}
	return client.GetGroupInfoFromLink(ctx, code)
}

func (s *WhatsAppService) JoinGroupWithLink(ctx context.Context, sessionID, code string) (string, error) {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return "", errors.New("session not connected")
	}
	return client.JoinGroupWithLink(ctx, code)
}

func (s *WhatsAppService) JoinGroupWithInvite(ctx context.Context, sessionID, groupJID, inviterJID, code string, expiration int64) error {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return errors.New("session not connected")
	}
	return client.JoinGroupWithInvite(ctx, groupJID, inviterJID, code, expiration)
}

func (s *WhatsAppService) CheckNumbers(ctx context.Context, sessionID string, phones []string) ([]domain.IsOnWhatsAppResult, error) {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return nil, errors.New("session not connected")
	}
	return client.IsOnWhatsApp(ctx, phones)
}

// ResolveContact resolves a normalized phone number to its canonical WhatsApp JID
// by querying IsOnWhatsApp in a single batch with Brazilian 9th-digit variations when applicable.
func (s *WhatsAppService) ResolveContact(ctx context.Context, sessionID, phone string) (*domain.ResolveContactResult, error) {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return nil, errors.New("session not connected")
	}

	candidates := buildPhoneVariations(phone)
	if len(candidates) == 0 {
		return nil, errors.New("invalid phone number")
	}

	results, err := client.IsOnWhatsApp(ctx, candidates)
	if err != nil {
		return nil, err
	}

	var fallback *domain.IsOnWhatsAppResult
	for i := range results {
		r := &results[i]
		if !r.IsIn || r.JID == "" {
			continue
		}
		if r.Phone == candidates[0] {
			return toResolveContactResult(r), nil
		}
		if fallback == nil {
			fallback = r
		}
	}

	if fallback != nil {
		return toResolveContactResult(fallback), nil
	}

	return &domain.ResolveContactResult{Exists: false}, nil
}

func toResolveContactResult(r *domain.IsOnWhatsAppResult) *domain.ResolveContactResult {
	phone := strings.TrimPrefix(r.Phone, "+")
	if phone == "" {
		phone = strings.Split(r.JID, "@")[0]
	}
	return &domain.ResolveContactResult{
		Exists: true,
		JID:    r.JID,
		Phone:  phone,
		Query:  r.Phone,
	}
}

// buildPhoneVariations normalizes a phone number to international "+<digits>" format
// and appends the Brazilian 9th-digit variation when applicable for mobile numbers.
func buildPhoneVariations(phone string) []string {
	digits := extractDigits(phone)
	if digits == "" {
		return nil
	}

	candidates := []string{"+" + digits}
	if strings.HasPrefix(digits, "55") {
		if len(digits) == 13 && digits[4] == '9' {
			candidates = append(candidates, "+"+digits[:4]+digits[5:])
		} else if len(digits) == 12 && digits[4] >= '6' && digits[4] <= '9' {
			candidates = append(candidates, "+"+digits[:4]+"9"+digits[4:])
		}
	}
	return candidates
}

func extractDigits(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func (s *WhatsAppService) GetUserInfo(ctx context.Context, sessionID, jid string) (*domain.UserInfoResult, error) {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return nil, errors.New("session not connected")
	}
	resMap, err := client.GetUserInfo(ctx, []string{jid})
	if err != nil {
		return nil, err
	}
	if info, exists := resMap[jid]; exists {
		return &info, nil
	}
	// fallback search without server part if needed
	for _, info := range resMap {
		return &info, nil
	}
	return &domain.UserInfoResult{JID: jid}, nil
}

func (s *WhatsAppService) GetContacts(ctx context.Context, sessionID string) ([]domain.ContactInfo, error) {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return nil, errors.New("session not connected")
	}
	return client.GetContacts(ctx)
}

func (s *WhatsAppService) GetBlocklist(ctx context.Context, sessionID string) ([]string, error) {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return nil, errors.New("session not connected")
	}
	return client.GetBlocklist(ctx)
}

func (s *WhatsAppService) UpdateBlocklist(ctx context.Context, sessionID, jid string, block bool) error {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return errors.New("session not connected")
	}
	return client.UpdateBlocklist(ctx, jid, block)
}

func (s *WhatsAppService) SetChatPin(ctx context.Context, sessionID, chatJID string, pin bool) error {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return errors.New("session not connected")
	}
	return client.SetChatPin(ctx, chatJID, pin)
}

func (s *WhatsAppService) SetChatMute(ctx context.Context, sessionID, chatJID string, mute bool, duration time.Duration) error {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return errors.New("session not connected")
	}
	return client.SetChatMute(ctx, chatJID, mute, duration)
}

func (s *WhatsAppService) SetChatArchive(ctx context.Context, sessionID, chatJID string, archive bool) error {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return errors.New("session not connected")
	}
	return client.SetChatArchive(ctx, chatJID, archive)
}

func (s *WhatsAppService) DeleteChat(ctx context.Context, sessionID, chatJID string) error {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return errors.New("session not connected")
	}
	return client.DeleteChat(ctx, chatJID)
}

func (s *WhatsAppService) CreateNewsletter(ctx context.Context, sessionID, name, description string, avatar []byte) (*domain.NewsletterInfo, error) {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return nil, errors.New("session not connected")
	}
	return client.CreateNewsletter(ctx, name, description, avatar)
}

func (s *WhatsAppService) GetSubscribedNewsletters(ctx context.Context, sessionID string) ([]domain.NewsletterInfo, error) {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return nil, errors.New("session not connected")
	}
	return client.GetSubscribedNewsletters(ctx)
}

func (s *WhatsAppService) FollowNewsletter(ctx context.Context, sessionID, jid string) error {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return errors.New("session not connected")
	}
	return client.FollowNewsletter(ctx, jid)
}

func (s *WhatsAppService) UnfollowNewsletter(ctx context.Context, sessionID, jid string) error {
	client, ok := s.clientRepo.GetClient(sessionID)
	if !ok || !client.IsLoggedIn() {
		return errors.New("session not connected")
	}
	return client.UnfollowNewsletter(ctx, jid)
}

func (s *WhatsAppService) formatJID(jid string, isGroup bool) string {
	if jid == "" {
		return ""
	}

	if strings.Contains(jid, "@") {
		return jid
	}

	if isGroup {
		return jid + "@g.us"
	}
	return jid + "@s.whatsapp.net"
}
