package handler

import (
	"chatnip/internal/kit/apierrors"
	httpPlatform "chatnip/internal/kit/http"
	"chatnip/internal/modules/whatsapp/domain"
	"chatnip/internal/modules/whatsapp/service"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type WhatsAppHandler struct {
	service *service.WhatsAppService
	logger  zerolog.Logger
}

func NewWhatsAppHandler(s *service.WhatsAppService, logger zerolog.Logger) *WhatsAppHandler {
	return &WhatsAppHandler{
		service: s,
		logger:  logger,
	}
}

// @Summary      Iniciar Sessão via QR Code
// @Description  Cria e inicia uma nova sessão de WhatsApp em segundo plano com o proxy de egressão obrigatório, retornando imediatamente o status PENDING_QR. O QR Code atualizado fica disponível em GET /api/v1/status.
// @Tags         WhatsApp - Sessões
// @Accept       json
// @Produce      json
// @Param        request body LoginRequest true "Dados de Inicialização da Sessão"
// @Success      200 {object} LoginResponse
// @Success      202 {object} LoginResponse
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/login [post]
func (h *WhatsAppHandler) handleLogin(w http.ResponseWriter, r *http.Request) error {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}

	err := h.service.Login(r.Context(), req.SessionID, req.TenantID, req.ProxyURL)
	if err != nil {
		if err == domain.ErrSessionAlreadyActive {
			httpPlatform.JSON(w, http.StatusOK, LoginResponse{
				Status: "connected",
				Msg:    "Session already active",
			})
			return nil
		}

		if err == domain.ErrProxyRequired {
			return apierrors.NewBadRequest("proxy_url is mandatory: direct egress connection is strictly forbidden")
		}

		return err
	}

	httpPlatform.JSON(w, http.StatusAccepted, LoginResponse{
		Status: "PENDING_QR",
		Msg:    "Session initialized. Check /api/v1/status for QR code.",
	})
	return nil
}

// @Summary      Desconectar Sessão (Logout)
// @Description  Encerra e desvincula a sessão ativa do WhatsApp.
// @Tags         WhatsApp - Sessões
// @Accept       json
// @Produce      plain
// @Param        request body LogoutRequest true "ID da Sessão"
// @Success      200 {string} string "Session ended"
// @Failure      400 {object} apierrors.AppError
// @Failure      404 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/logout [post]
func (h *WhatsAppHandler) handleLogout(w http.ResponseWriter, r *http.Request) error {
	var req LogoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}

	if err := h.service.Logout(r.Context(), req.SessionID); err != nil {
		if err == domain.ErrSessionNotFound {
			return apierrors.NewNotFound(err.Error())
		}
		return err
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Session ended"))
	return nil
}

// @Summary      Obter Status da Sessão
// @Description  Retorna o estado de conectividade, login e o último QR Code disponível (quando pendente de pareamento) de uma sessão do WhatsApp.
// @Tags         WhatsApp - Sessões
// @Produce      json
// @Param        session_id query string true "ID da Sessão"
// @Success      200 {object} domain.SessionStatus
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/status [get]
func (h *WhatsAppHandler) handleStatus(w http.ResponseWriter, r *http.Request) error {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		return apierrors.NewBadRequest("session_id required")
	}

	status, err := h.service.GetStatus(sessionID)
	if err != nil {
		return err
	}

	httpPlatform.JSON(w, http.StatusOK, status)
	return nil
}

// @Summary      Listar Sessões
// @Description  Retorna todas as sessões ou filtra por tenant.
// @Tags         WhatsApp - Sessões
// @Produce      json
// @Param        tenant_id query string false "ID do Tenant"
// @Success      200 {array} domain.SessionStatus
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/sessions [get]
func (h *WhatsAppHandler) handleListSessions(w http.ResponseWriter, r *http.Request) error {
	tenantID := r.URL.Query().Get("tenant_id")

	sessions, err := h.service.ListSessions(r.Context(), tenantID)
	if err != nil {
		return apierrors.NewInternalServerError("failed to fetch sessions")
	}

	httpPlatform.JSON(w, http.StatusOK, sessions)
	return nil
}

// @Summary      Enviar Mensagem (Texto ou Mídia)
// @Description  Envia mensagens de texto ou mídias via fila RabbitMQ com simulação humanizada (HSEM v3) ou pista rápida.
// @Tags         WhatsApp - Mensagens
// @Accept       json
// @Produce      json
// @Param        request body SendMessageRequest true "Dados do Envio (Texto ou Mídia)"
// @Success      202 {object} map[string]string "Status de fila (queued)"
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/send [post]
func (h *WhatsAppHandler) handleSend(w http.ResponseWriter, r *http.Request) error {
	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}

	if req.RemoteJID == "" {
		return apierrors.NewBadRequest("jid is required")
	}

	if status, _ := h.service.GetStatus(req.SessionID); !status.LoggedIn {
		return apierrors.NewBadRequest("session not connected")
	}

	payload := domain.SendMessagePayload{
		CorrelationID:     req.CorrelationID,
		SessionID:         req.SessionID,
		RemoteJID:         req.RemoteJID,
		Text:              req.Message,
		IsGroup:           req.IsGroup,
		IsPriority:        req.IsPriority,
		MediaURL:          req.MediaURL,
		MediaType:         string(req.MediaType),
		IsPTT:             req.IsPTT,
		IsGIF:             req.IsGIF,
		FileName:          req.FileName,
		QuotedMsgID:       req.QuotedMsgID,
		QuotedParticipant: req.QuotedParticipant,
	}

	if err := h.service.QueueMessage(r.Context(), payload); err != nil {
		h.logger.Error().Err(err).Str("session_id", req.SessionID).Msg("Failed to queue message")
		return apierrors.NewInternalServerError("Failed to queue message")
	}

	queueType := "standard_lane"
	if req.IsPriority {
		queueType = "fast_lane"
	}

	httpPlatform.JSON(w, http.StatusAccepted, map[string]string{
		"status":   "queued",
		"queue":    queueType,
		"strategy": "humanized_hsem_v3",
	})
	return nil
}

// @Summary      Editar Mensagem
// @Description  Edita o texto de uma mensagem enviada anteriormente.
// @Tags         WhatsApp - Mensagens
// @Accept       json
// @Produce      json
// @Param        request body EditMessageRequest true "Dados da Edição"
// @Success      202 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/messages/edit [post]
func (h *WhatsAppHandler) handleEditMessage(w http.ResponseWriter, r *http.Request) error {
	var req EditMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}

	if req.SessionID == "" || req.RemoteJID == "" || req.MessageID == "" || req.NewMessage == "" {
		return apierrors.NewBadRequest("session_id, jid, message_id and new_message are required")
	}

	if status, _ := h.service.GetStatus(req.SessionID); !status.LoggedIn {
		return apierrors.NewBadRequest("session not connected")
	}

	payload := domain.SendMessagePayload{
		CorrelationID: req.CorrelationID,
		SessionID:     req.SessionID,
		RemoteJID:     req.RemoteJID,
		Text:          req.NewMessage,
		Action:        "edit",
		TargetMsgID:   req.MessageID,
		IsGroup:       req.IsGroup,
		IsPriority:    req.IsPriority,
	}

	if err := h.service.QueueMessage(r.Context(), payload); err != nil {
		h.logger.Error().Err(err).Str("session_id", req.SessionID).Msg("Failed to queue edit message")
		return apierrors.NewInternalServerError("Failed to queue edit message")
	}

	httpPlatform.JSON(w, http.StatusAccepted, map[string]string{
		"status": "queued",
		"action": "edit",
	})
	return nil
}

// @Summary      Reagir a Mensagem
// @Description  Envia uma reação com emoji a uma mensagem (ou remove com emoji vazio).
// @Tags         WhatsApp - Mensagens
// @Accept       json
// @Produce      json
// @Param        request body ReactionMessageRequest true "Dados da Reação"
// @Success      202 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/messages/reaction [post]
func (h *WhatsAppHandler) handleReactionMessage(w http.ResponseWriter, r *http.Request) error {
	var req ReactionMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}

	if req.SessionID == "" || req.RemoteJID == "" || req.MessageID == "" {
		return apierrors.NewBadRequest("session_id, jid and message_id are required")
	}

	if status, _ := h.service.GetStatus(req.SessionID); !status.LoggedIn {
		return apierrors.NewBadRequest("session not connected")
	}

	payload := domain.SendMessagePayload{
		CorrelationID:     req.CorrelationID,
		SessionID:         req.SessionID,
		RemoteJID:         req.RemoteJID,
		Action:            "reaction",
		Reaction:          req.Reaction,
		TargetMsgID:       req.MessageID,
		TargetParticipant: req.SenderJID,
		IsGroup:           req.IsGroup,
		IsPriority:        req.IsPriority,
	}

	if err := h.service.QueueMessage(r.Context(), payload); err != nil {
		h.logger.Error().Err(err).Str("session_id", req.SessionID).Msg("Failed to queue reaction")
		return apierrors.NewInternalServerError("Failed to queue reaction")
	}

	httpPlatform.JSON(w, http.StatusAccepted, map[string]string{
		"status": "queued",
		"action": "reaction",
	})
	return nil
}

// @Summary      Revogar Mensagem
// @Description  Revoga (deleta para todos) uma mensagem enviada.
// @Tags         WhatsApp - Mensagens
// @Accept       json
// @Produce      json
// @Param        request body RevokeMessageRequest true "Dados da Revogação"
// @Success      202 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/messages/revoke [post]
func (h *WhatsAppHandler) handleRevokeMessage(w http.ResponseWriter, r *http.Request) error {
	var req RevokeMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}

	if req.SessionID == "" || req.RemoteJID == "" || req.MessageID == "" {
		return apierrors.NewBadRequest("session_id, jid and message_id are required")
	}

	if status, _ := h.service.GetStatus(req.SessionID); !status.LoggedIn {
		return apierrors.NewBadRequest("session not connected")
	}

	payload := domain.SendMessagePayload{
		CorrelationID:     req.CorrelationID,
		SessionID:         req.SessionID,
		RemoteJID:         req.RemoteJID,
		Action:            "revoke",
		TargetMsgID:       req.MessageID,
		TargetParticipant: req.SenderJID,
		IsGroup:           req.IsGroup,
		IsPriority:        req.IsPriority,
	}

	if err := h.service.QueueMessage(r.Context(), payload); err != nil {
		h.logger.Error().Err(err).Str("session_id", req.SessionID).Msg("Failed to queue revoke message")
		return apierrors.NewInternalServerError("Failed to queue revoke message")
	}

	httpPlatform.JSON(w, http.StatusAccepted, map[string]string{
		"status": "queued",
		"action": "revoke",
	})
	return nil
}

// @Summary      Atualizar Proxy da Sessão
// @Description  Atualiza o endereço de proxy de uma sessão existente e reconecta o cliente se necessário.
// @Tags         WhatsApp - Sessões
// @Accept       json
// @Produce      json
// @Param        request body UpdateProxyRequest true "Dados de Atualização"
// @Success      200 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      404 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/proxy [put]
func (h *WhatsAppHandler) handleUpdateProxy(w http.ResponseWriter, r *http.Request) error {
	var req UpdateProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}

	if req.SessionID == "" {
		return apierrors.NewBadRequest("session_id is required")
	}

	if err := h.service.UpdateProxy(r.Context(), req.SessionID, req.ProxyURL); err != nil {
		if err == domain.ErrSessionNotFound {
			return apierrors.NewNotFound("session not found")
		}
		if err == domain.ErrProxyRequired {
			return apierrors.NewBadRequest("proxy_url cannot be empty: direct egress connection is strictly forbidden")
		}
		return err
	}

	httpPlatform.JSON(w, http.StatusOK, map[string]string{
		"status": "updated",
		"msg":    "Proxy updated and client reloaded",
	})
	return nil
}

// @Summary      Validar Proxy de Egressão
// @Description  Testa a conectividade, autenticação e IP de saída de uma URL de proxy (HTTP/HTTPS/SOCKS5) antes de criar ou atualizar uma sessão.
// @Tags         WhatsApp - Sessões
// @Accept       json
// @Produce      json
// @Param        request body ValidateProxyRequest true "URL do Proxy"
// @Success      200 {object} domain.ProxyValidationResult
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/proxy/validate [post]
func (h *WhatsAppHandler) handleValidateProxy(w http.ResponseWriter, r *http.Request) error {
	var req ValidateProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}

	result, err := h.service.ValidateProxy(r.Context(), req.ProxyURL)
	if err != nil {
		if err == domain.ErrProxyRequired {
			return apierrors.NewBadRequest("proxy_url is required")
		}
		return err
	}

	httpPlatform.JSON(w, http.StatusOK, result)
	return nil
}

// @Summary      Criar Grupo
// @Description  Cria um novo grupo com os participantes informados.
// @Tags         WhatsApp - Grupos
// @Accept       json
// @Produce      json
// @Param        request body CreateGroupRequest true "Dados do Grupo"
// @Success      201 {object} CreateGroupResponse
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/groups [post]
func (h *WhatsAppHandler) handleCreateGroup(w http.ResponseWriter, r *http.Request) error {
	var req CreateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}

	if req.SessionID == "" || req.Name == "" {
		return apierrors.NewBadRequest("session_id and name are required")
	}

	if len(req.Participants) == 0 {
		return apierrors.NewBadRequest("participants list cannot be empty")
	}

	groupID, err := h.service.CreateGroup(r.Context(), req.SessionID, req.Name, req.Participants)
	if err != nil {
		return apierrors.NewInternalServerError(fmt.Sprintf("failed to create group: %v", err))
	}

	httpPlatform.JSON(w, http.StatusCreated, CreateGroupResponse{
		GroupID: groupID,
		Msg:     "Group created successfully",
	})
	return nil
}

// @Summary      Listar Grupos
// @Description  Retorna todos os grupos que a sessão participa.
// @Tags         WhatsApp - Grupos
// @Produce      json
// @Param        session_id query string true "Session ID"
// @Success      200 {object} ListGroupsResponse
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/groups [get]
func (h *WhatsAppHandler) handleListGroups(w http.ResponseWriter, r *http.Request) error {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		return apierrors.NewBadRequest("session_id is required")
	}

	groups, err := h.service.ListGroups(r.Context(), sessionID)
	if err != nil {
		return apierrors.NewInternalServerError(fmt.Sprintf("failed to list groups: %v", err))
	}

	httpPlatform.JSON(w, http.StatusOK, ListGroupsResponse{
		SessionID: sessionID,
		Groups:    groups,
	})
	return nil
}

// @Summary      Configurar Quem Pode Enviar Mensagens no Grupo
// @Description  Define se todos os participantes ou apenas administradores podem enviar mensagens no grupo.
// @Tags         WhatsApp - Grupos
// @Accept       json
// @Produce      json
// @Param        request body SetGroupMessageSettingRequest true "Configuração"
// @Success      200 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/groups/settings/messages [put]
func (h *WhatsAppHandler) handleSetGroupMessageSetting(w http.ResponseWriter, r *http.Request) error {
	var req SetGroupMessageSettingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}

	if req.SessionID == "" || req.GroupJID == "" {
		return apierrors.NewBadRequest("session_id and group_jid are required")
	}

	err := h.service.SetGroupMessageSetting(r.Context(), req.SessionID, req.GroupJID, req.OnlyAdmins)
	if err != nil {
		return apierrors.NewInternalServerError(fmt.Sprintf("failed to update group settings: %v", err))
	}

	msg := "Group opened to all participants"
	if req.OnlyAdmins {
		msg = "Group restricted to admins only"
	}

	httpPlatform.JSON(w, http.StatusOK, map[string]string{
		"status": "updated",
		"msg":    msg,
	})
	return nil
}

// @Summary      Iniciar Sessão via Código de Pareamento
// @Description  Gera um código de 8 dígitos para vincular o WhatsApp no celular sem usar QR Code.
// @Tags         WhatsApp - Sessões
// @Accept       json
// @Produce      json
// @Param        request body LoginPairCodeRequest true "Dados de Pareamento com Telefone"
// @Success      200 {object} LoginPairCodeResponse
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/login/pair-code [post]
func (h *WhatsAppHandler) handleLoginPairCode(w http.ResponseWriter, r *http.Request) error {
	var req LoginPairCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}

	if req.Phone == "" || req.SessionID == "" {
		return apierrors.NewBadRequest("phone and session_id are required")
	}

	code, err := h.service.LoginWithPairCode(r.Context(), req.SessionID, req.TenantID, req.Phone, req.ProxyURL)
	if err != nil {
		if err == domain.ErrSessionAlreadyActive {
			httpPlatform.JSON(w, http.StatusOK, LoginPairCodeResponse{
				Status: "already_active",
			})
			return nil
		}
		if err == domain.ErrProxyRequired {
			return apierrors.NewBadRequest("proxy_url is mandatory: direct egress connection is strictly forbidden")
		}
		return apierrors.NewInternalServerError(err.Error())
	}

	httpPlatform.JSON(w, http.StatusOK, LoginPairCodeResponse{
		Status:   "pair_code_generated",
		PairCode: code,
	})
	return nil
}

// @Summary      Marcar Mensagem como Lida
// @Description  Enfileira a confirmação de leitura no RabbitMQ com execução humanizada pelo motor.
// @Tags         WhatsApp - Mensagens
// @Accept       json
// @Produce      json
// @Param        request body ReadMessageRequest true "Dados da Mensagem"
// @Success      202 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/messages/read [post]
func (h *WhatsAppHandler) handleReadMessage(w http.ResponseWriter, r *http.Request) error {
	var req ReadMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}

	if req.SessionID == "" || req.ChatJID == "" || req.MessageID == "" {
		return apierrors.NewBadRequest("session_id, chat_jid and message_id are required")
	}

	correlationID := req.CorrelationID
	if correlationID == "" {
		correlationID = uuid.New().String()
	}

	payload := domain.SendMessagePayload{
		CorrelationID: correlationID,
		SessionID:     req.SessionID,
		RemoteJID:     req.ChatJID,
		Action:        "read",
		TargetMsgID:   req.MessageID,
		IsGroup:       req.IsGroup,
		IsPriority:    true,
	}

	if err := h.service.QueueMessage(r.Context(), payload); err != nil {
		h.logger.Error().Err(err).Str("session_id", req.SessionID).Msg("Failed to queue mark as read")
		return apierrors.NewInternalServerError("Failed to queue mark as read")
	}

	httpPlatform.JSON(w, http.StatusAccepted, map[string]string{
		"status": "queued",
		"action": "read",
	})
	return nil
}

// @Summary      Publicar Status / Stories
// @Description  Publica uma atualização de texto ou mídia no Status do WhatsApp via fila e despacho humanizado.
// @Tags         WhatsApp - Status
// @Accept       json
// @Produce      json
// @Param        request body SendStatusRequest true "Dados do Status"
// @Success      202 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/status/send [post]
func (h *WhatsAppHandler) handleSendStatus(w http.ResponseWriter, r *http.Request) error {
	var req SendStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}

	if req.SessionID == "" {
		return apierrors.NewBadRequest("session_id is required")
	}

	correlationID := req.CorrelationID
	if correlationID == "" {
		correlationID = uuid.New().String()
	}

	payload := domain.SendMessagePayload{
		CorrelationID: correlationID,
		SessionID:     req.SessionID,
		RemoteJID:     "status@broadcast",
		Text:          req.Message,
		MediaURL:      req.MediaURL,
		MediaType:     string(req.MediaType),
		Action:        "status",
		IsPriority:    false,
	}

	if err := h.service.QueueMessage(r.Context(), payload); err != nil {
		h.logger.Error().Err(err).Str("session_id", req.SessionID).Msg("Failed to queue status message")
		return apierrors.NewInternalServerError("Failed to queue status message")
	}

	httpPlatform.JSON(w, http.StatusAccepted, map[string]string{
		"status":   "queued",
		"target":   "status@broadcast",
		"strategy": "humanized_hsem_v3",
	})
	return nil
}

// @Summary      Obter Informações do Grupo
// @Description  Recupera metadados completos do grupo, incluindo assunto, descrição, criador e lista de participantes.
// @Tags         WhatsApp - Grupos
// @Produce      json
// @Param        jid path string true "JID do Grupo"
// @Param        session_id query string true "ID da Sessão"
// @Success      200 {object} domain.GroupMetadata
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/groups/{jid} [get]
func (h *WhatsAppHandler) handleGetGroupInfo(w http.ResponseWriter, r *http.Request) error {
	groupJID := chi.URLParam(r, "jid")
	sessionID := r.URL.Query().Get("session_id")
	if groupJID == "" || sessionID == "" {
		return apierrors.NewBadRequest("jid and session_id query param are required")
	}

	info, err := h.service.GetGroupInfo(r.Context(), sessionID, groupJID)
	if err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}

	httpPlatform.JSON(w, http.StatusOK, info)
	return nil
}

// @Summary      Alterar Nome do Grupo
// @Description  Atualiza o nome (assunto) de um grupo do WhatsApp.
// @Tags         WhatsApp - Grupos
// @Accept       json
// @Produce      json
// @Param        jid path string true "JID do Grupo"
// @Param        request body SetGroupNameRequest true "Novo Nome do Grupo"
// @Success      200 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/groups/{jid}/name [put]
func (h *WhatsAppHandler) handleSetGroupName(w http.ResponseWriter, r *http.Request) error {
	groupJID := chi.URLParam(r, "jid")
	var req SetGroupNameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}
	if req.GroupJID == "" {
		req.GroupJID = groupJID
	}
	if req.SessionID == "" || req.GroupJID == "" || req.Name == "" {
		return apierrors.NewBadRequest("session_id, group_jid and name are required")
	}
	if err := h.service.SetGroupName(r.Context(), req.SessionID, req.GroupJID, req.Name); err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, map[string]string{"status": "updated", "name": req.Name})
	return nil
}

// @Summary      Alterar Descrição do Grupo
// @Description  Atualiza o tópico (descrição) de um grupo do WhatsApp.
// @Tags         WhatsApp - Grupos
// @Accept       json
// @Produce      json
// @Param        jid path string true "JID do Grupo"
// @Param        request body SetGroupTopicRequest true "Nova Descrição do Grupo"
// @Success      200 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/groups/{jid}/topic [put]
func (h *WhatsAppHandler) handleSetGroupTopic(w http.ResponseWriter, r *http.Request) error {
	groupJID := chi.URLParam(r, "jid")
	var req SetGroupTopicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}
	if req.GroupJID == "" {
		req.GroupJID = groupJID
	}
	if req.SessionID == "" || req.GroupJID == "" {
		return apierrors.NewBadRequest("session_id and group_jid are required")
	}
	if err := h.service.SetGroupTopic(r.Context(), req.SessionID, req.GroupJID, req.Topic); err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, map[string]string{"status": "updated", "topic": req.Topic})
	return nil
}

// @Summary      Definir Foto do Grupo
// @Description  Define uma nova imagem de perfil do grupo utilizando Base64 (formato JPEG).
// @Tags         WhatsApp - Grupos
// @Accept       json
// @Produce      json
// @Param        jid path string true "JID do Grupo"
// @Param        request body SetGroupPhotoRequest true "Foto em Base64"
// @Success      200 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/groups/{jid}/photo [put]
func (h *WhatsAppHandler) handleSetGroupPhoto(w http.ResponseWriter, r *http.Request) error {
	groupJID := chi.URLParam(r, "jid")
	var req SetGroupPhotoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}
	if req.GroupJID == "" {
		req.GroupJID = groupJID
	}
	if req.SessionID == "" || req.GroupJID == "" || req.PhotoBase64 == "" {
		return apierrors.NewBadRequest("session_id, group_jid and photo_base64 are required")
	}

	data, err := base64.StdEncoding.DecodeString(req.PhotoBase64)
	if err != nil {
		return apierrors.NewBadRequest("invalid base64 image")
	}

	picID, err := h.service.SetGroupPhoto(r.Context(), req.SessionID, req.GroupJID, data)
	if err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, map[string]string{"status": "updated", "picture_id": picID})
	return nil
}

// @Summary      Remover Foto do Grupo
// @Description  Exclui o avatar atual do grupo.
// @Tags         WhatsApp - Grupos
// @Produce      json
// @Param        jid path string true "JID do Grupo"
// @Param        session_id query string true "ID da Sessão"
// @Success      200 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/groups/{jid}/photo [delete]
func (h *WhatsAppHandler) handleDeleteGroupPhoto(w http.ResponseWriter, r *http.Request) error {
	groupJID := chi.URLParam(r, "jid")
	sessionID := r.URL.Query().Get("session_id")
	if groupJID == "" || sessionID == "" {
		return apierrors.NewBadRequest("jid and session_id are required")
	}
	if _, err := h.service.SetGroupPhoto(r.Context(), sessionID, groupJID, nil); err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, map[string]string{"status": "removed"})
	return nil
}

// @Summary      Configurar Edição de Dados do Grupo
// @Description  Define se apenas administradores podem alterar o nome, ícone e descrição do grupo.
// @Tags         WhatsApp - Grupos
// @Accept       json
// @Produce      json
// @Param        jid path string true "JID do Grupo"
// @Param        request body SetGroupInfoSettingRequest true "Configuração de Bloqueio"
// @Success      200 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/groups/{jid}/settings/info [put]
func (h *WhatsAppHandler) handleSetGroupInfoSetting(w http.ResponseWriter, r *http.Request) error {
	groupJID := chi.URLParam(r, "jid")
	var req SetGroupInfoSettingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}
	if req.GroupJID == "" {
		req.GroupJID = groupJID
	}
	if req.SessionID == "" || req.GroupJID == "" {
		return apierrors.NewBadRequest("session_id and group_jid are required")
	}
	if err := h.service.SetGroupInfoLocked(r.Context(), req.SessionID, req.GroupJID, req.OnlyAdmins); err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, map[string]string{"status": "updated", "locked": fmt.Sprintf("%v", req.OnlyAdmins)})
	return nil
}

// @Summary      Gerenciar Participantes do Grupo
// @Description  Adiciona, remove, promove a administrador ou rebaixa participantes do grupo (ações: add, remove, promote, demote).
// @Tags         WhatsApp - Grupos
// @Accept       json
// @Produce      json
// @Param        jid path string true "JID do Grupo"
// @Param        request body UpdateGroupParticipantsRequest true "Dados dos Participantes e Ação"
// @Success      200 {object} map[string]interface{}
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/groups/{jid}/participants [post]
func (h *WhatsAppHandler) handleUpdateGroupParticipants(w http.ResponseWriter, r *http.Request) error {
	groupJID := chi.URLParam(r, "jid")
	var req UpdateGroupParticipantsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}
	if req.GroupJID == "" {
		req.GroupJID = groupJID
	}
	if req.SessionID == "" || req.GroupJID == "" || len(req.Participants) == 0 || req.Action == "" {
		return apierrors.NewBadRequest("session_id, group_jid, participants and action are required")
	}
	updated, err := h.service.UpdateGroupParticipants(r.Context(), req.SessionID, req.GroupJID, req.Participants, req.Action)
	if err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, map[string]interface{}{"status": "success", "updated": updated})
	return nil
}

// @Summary      Sair do Grupo
// @Description  Desconecta a sessão e sai definitivamente de um grupo do WhatsApp.
// @Tags         WhatsApp - Grupos
// @Accept       json
// @Produce      json
// @Param        jid path string true "JID do Grupo"
// @Param        request body LeaveGroupRequest false "ID da Sessão"
// @Param        session_id query string false "ID da Sessão (alternativo via query)"
// @Success      200 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/groups/{jid}/leave [post]
func (h *WhatsAppHandler) handleLeaveGroup(w http.ResponseWriter, r *http.Request) error {
	groupJID := chi.URLParam(r, "jid")
	var req LeaveGroupRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.GroupJID == "" {
		req.GroupJID = groupJID
	}
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = r.URL.Query().Get("session_id")
	}
	if sessionID == "" || req.GroupJID == "" {
		return apierrors.NewBadRequest("session_id and group_jid are required")
	}
	if err := h.service.LeaveGroup(r.Context(), sessionID, req.GroupJID); err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, map[string]string{"status": "left"})
	return nil
}

// @Summary      Obter Link de Convite do Grupo
// @Description  Gera ou recupera o link de convite oficial do grupo (chat.whatsapp.com/...).
// @Tags         WhatsApp - Grupos
// @Produce      json
// @Param        jid path string true "JID do Grupo"
// @Param        session_id query string true "ID da Sessão"
// @Success      200 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/groups/{jid}/invite [get]
func (h *WhatsAppHandler) handleGetGroupInviteLink(w http.ResponseWriter, r *http.Request) error {
	groupJID := chi.URLParam(r, "jid")
	sessionID := r.URL.Query().Get("session_id")
	if groupJID == "" || sessionID == "" {
		return apierrors.NewBadRequest("jid and session_id are required")
	}
	link, err := h.service.GetGroupInviteLink(r.Context(), sessionID, groupJID, false)
	if err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, map[string]string{"invite_link": link})
	return nil
}

// @Summary      Revogar Link de Convite do Grupo
// @Description  Invalida o link de convite atual do grupo e gera imediatamente um novo.
// @Tags         WhatsApp - Grupos
// @Produce      json
// @Param        jid path string true "JID do Grupo"
// @Param        session_id query string true "ID da Sessão"
// @Success      200 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/groups/{jid}/invite/revoke [post]
func (h *WhatsAppHandler) handleRevokeGroupInviteLink(w http.ResponseWriter, r *http.Request) error {
	groupJID := chi.URLParam(r, "jid")
	sessionID := r.URL.Query().Get("session_id")
	if groupJID == "" || sessionID == "" {
		return apierrors.NewBadRequest("jid and session_id are required")
	}
	link, err := h.service.GetGroupInviteLink(r.Context(), sessionID, groupJID, true)
	if err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, map[string]string{"new_invite_link": link})
	return nil
}

// @Summary      Inspecionar Grupo por Código de Convite
// @Description  Retorna metadados básicos de um grupo através do seu código de convite sem entrar no grupo.
// @Tags         WhatsApp - Grupos
// @Produce      json
// @Param        code path string true "Código do Convite"
// @Param        session_id query string true "ID da Sessão"
// @Success      200 {object} domain.GroupInviteInfo
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/groups/invites/{code} [get]
func (h *WhatsAppHandler) handleGetGroupInfoFromInvite(w http.ResponseWriter, r *http.Request) error {
	code := chi.URLParam(r, "code")
	sessionID := r.URL.Query().Get("session_id")
	if code == "" || sessionID == "" {
		return apierrors.NewBadRequest("code and session_id are required")
	}
	info, err := h.service.GetGroupInfoFromLink(r.Context(), sessionID, code)
	if err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, info)
	return nil
}

// @Summary      Entrar em Grupo via Link ou Código
// @Description  Entra em um grupo do WhatsApp utilizando o link ou código de convite.
// @Tags         WhatsApp - Grupos
// @Accept       json
// @Produce      json
// @Param        request body JoinGroupLinkRequest true "Código ou Link do Convite"
// @Success      200 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/groups/join [post]
func (h *WhatsAppHandler) handleJoinGroup(w http.ResponseWriter, r *http.Request) error {
	var req JoinGroupLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}
	if req.SessionID == "" || req.Code == "" {
		return apierrors.NewBadRequest("session_id and code are required")
	}
	code := req.Code
	if strings.Contains(code, "/") {
		parts := strings.Split(code, "/")
		code = parts[len(parts)-1]
	}
	groupJID, err := h.service.JoinGroupWithLink(r.Context(), req.SessionID, code)
	if err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, map[string]string{"status": "joined", "group_jid": groupJID})
	return nil
}

// @Summary      Entrar em Grupo via Mensagem de Convite
// @Description  Entra em um grupo utilizando uma mensagem de convite privada enviada por um contato.
// @Tags         WhatsApp - Grupos
// @Accept       json
// @Produce      json
// @Param        request body JoinGroupInviteMessageRequest true "Dados do Convite Privado"
// @Success      200 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/groups/join-invite [post]
func (h *WhatsAppHandler) handleJoinGroupInviteMessage(w http.ResponseWriter, r *http.Request) error {
	var req JoinGroupInviteMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}
	if req.SessionID == "" || req.GroupJID == "" || req.InviterJID == "" || req.Code == "" {
		return apierrors.NewBadRequest("session_id, group_jid, inviter_jid and code are required")
	}
	if err := h.service.JoinGroupWithInvite(r.Context(), req.SessionID, req.GroupJID, req.InviterJID, req.Code, req.Expiration); err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, map[string]string{"status": "joined"})
	return nil
}

// @Summary      Validar Números no WhatsApp
// @Description  Verifica em lote se uma lista de números de telefone possui contas ativas no WhatsApp.
// @Tags         WhatsApp - Contatos
// @Accept       json
// @Produce      json
// @Param        request body CheckNumbersRequest true "Lista de Telefones com DDI"
// @Success      200 {array} domain.IsOnWhatsAppResult
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/contacts/check [post]
func (h *WhatsAppHandler) handleCheckNumbers(w http.ResponseWriter, r *http.Request) error {
	var req CheckNumbersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}
	if req.SessionID == "" || len(req.Phones) == 0 {
		return apierrors.NewBadRequest("session_id and phones are required")
	}
	results, err := h.service.CheckNumbers(r.Context(), req.SessionID, req.Phones)
	if err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, results)
	return nil
}

// @Summary      Listar Contatos Salvos
// @Description  Retorna a lista de contatos em cache sincronizados na base SQLite local do dispositivo.
// @Tags         WhatsApp - Contatos
// @Produce      json
// @Param        session_id query string true "ID da Sessão"
// @Success      200 {array} domain.ContactInfo
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/contacts [get]
func (h *WhatsAppHandler) handleListContacts(w http.ResponseWriter, r *http.Request) error {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		return apierrors.NewBadRequest("session_id is required")
	}
	contacts, err := h.service.GetContacts(r.Context(), sessionID)
	if err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, contacts)
	return nil
}

// @Summary      Obter Informações de Contato
// @Description  Busca sob demanda os dados de perfil (avatar, status/recado e nome verificado) de um contato.
// @Tags         WhatsApp - Contatos
// @Produce      json
// @Param        jid path string true "JID do Contato"
// @Param        session_id query string true "ID da Sessão"
// @Success      200 {object} domain.UserInfoResult
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/contacts/{jid}/info [get]
func (h *WhatsAppHandler) handleGetUserInfo(w http.ResponseWriter, r *http.Request) error {
	jid := chi.URLParam(r, "jid")
	sessionID := r.URL.Query().Get("session_id")
	if jid == "" || sessionID == "" {
		return apierrors.NewBadRequest("jid and session_id are required")
	}
	info, err := h.service.GetUserInfo(r.Context(), sessionID, jid)
	if err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, info)
	return nil
}

// @Summary      Fixar / Desafixar Conversa
// @Description  Fixa ou desafixa um chat no topo da lista do WhatsApp.
// @Tags         WhatsApp - Conversas
// @Accept       json
// @Produce      json
// @Param        jid path string true "JID do Chat"
// @Param        request body SetChatPinRequest true "Estado de Fixação"
// @Success      200 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/chats/{jid}/pin [put]
func (h *WhatsAppHandler) handleSetChatPin(w http.ResponseWriter, r *http.Request) error {
	chatJID := chi.URLParam(r, "jid")
	var req SetChatPinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}
	if req.ChatJID == "" {
		req.ChatJID = chatJID
	}
	if req.SessionID == "" || req.ChatJID == "" {
		return apierrors.NewBadRequest("session_id and chat_jid are required")
	}
	if err := h.service.SetChatPin(r.Context(), req.SessionID, req.ChatJID, req.Pin); err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, map[string]string{"status": "updated", "pin": fmt.Sprintf("%v", req.Pin)})
	return nil
}

// @Summary      Silenciar / Reativar Conversa
// @Description  Silencia ou reativa notificações de um chat com duração programada (em segundos).
// @Tags         WhatsApp - Conversas
// @Accept       json
// @Produce      json
// @Param        jid path string true "JID do Chat"
// @Param        request body SetChatMuteRequest true "Configuração de Silenciamento"
// @Success      200 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/chats/{jid}/mute [put]
func (h *WhatsAppHandler) handleSetChatMute(w http.ResponseWriter, r *http.Request) error {
	chatJID := chi.URLParam(r, "jid")
	var req SetChatMuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}
	if req.ChatJID == "" {
		req.ChatJID = chatJID
	}
	if req.SessionID == "" || req.ChatJID == "" {
		return apierrors.NewBadRequest("session_id and chat_jid are required")
	}
	dur := time.Duration(req.DurationSeconds) * time.Second
	if req.Mute && dur == 0 {
		dur = 365 * 24 * time.Hour
	}
	if err := h.service.SetChatMute(r.Context(), req.SessionID, req.ChatJID, req.Mute, dur); err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, map[string]string{"status": "updated", "mute": fmt.Sprintf("%v", req.Mute)})
	return nil
}

// @Summary      Arquivar / Desarquivar Conversa
// @Description  Move o chat para a pasta de arquivados ou restaura para a lista principal.
// @Tags         WhatsApp - Conversas
// @Accept       json
// @Produce      json
// @Param        jid path string true "JID do Chat"
// @Param        request body SetChatArchiveRequest true "Estado de Arquivamento"
// @Success      200 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/chats/{jid}/archive [put]
func (h *WhatsAppHandler) handleSetChatArchive(w http.ResponseWriter, r *http.Request) error {
	chatJID := chi.URLParam(r, "jid")
	var req SetChatArchiveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}
	if req.ChatJID == "" {
		req.ChatJID = chatJID
	}
	if req.SessionID == "" || req.ChatJID == "" {
		return apierrors.NewBadRequest("session_id and chat_jid are required")
	}
	if err := h.service.SetChatArchive(r.Context(), req.SessionID, req.ChatJID, req.Archive); err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, map[string]string{"status": "updated", "archive": fmt.Sprintf("%v", req.Archive)})
	return nil
}

// @Summary      Limpar Conversa
// @Description  Limpa o histórico de mensagens da conversa na conta do WhatsApp.
// @Tags         WhatsApp - Conversas
// @Produce      json
// @Param        jid path string true "JID do Chat"
// @Param        session_id query string true "ID da Sessão"
// @Success      200 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/chats/{jid} [delete]
func (h *WhatsAppHandler) handleDeleteChat(w http.ResponseWriter, r *http.Request) error {
	chatJID := chi.URLParam(r, "jid")
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		var req struct {
			SessionID string `json:"session_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		sessionID = req.SessionID
	}
	if sessionID == "" || chatJID == "" {
		return apierrors.NewBadRequest("session_id and chat_jid are required")
	}
	if err := h.service.DeleteChat(r.Context(), sessionID, chatJID); err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	return nil
}

// @Summary      Listar Contatos Bloqueados
// @Description  Retorna a lista de JIDs que foram bloqueados pela conta do WhatsApp.
// @Tags         WhatsApp - Contatos
// @Produce      json
// @Param        session_id query string true "ID da Sessão"
// @Success      200 {array} string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/blocklist [get]
func (h *WhatsAppHandler) handleGetBlocklist(w http.ResponseWriter, r *http.Request) error {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		return apierrors.NewBadRequest("session_id is required")
	}
	list, err := h.service.GetBlocklist(r.Context(), sessionID)
	if err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, list)
	return nil
}

// @Summary      Bloquear / Desbloquear Contato
// @Description  Adiciona ou remove um número de telefone da lista de bloqueio do WhatsApp.
// @Tags         WhatsApp - Contatos
// @Accept       json
// @Produce      json
// @Param        request body BlocklistRequest true "Dados do Bloqueio"
// @Success      200 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/blocklist [post]
func (h *WhatsAppHandler) handleUpdateBlocklist(w http.ResponseWriter, r *http.Request) error {
	var req BlocklistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}
	if req.SessionID == "" || req.JID == "" {
		return apierrors.NewBadRequest("session_id and jid are required")
	}
	if err := h.service.UpdateBlocklist(r.Context(), req.SessionID, req.JID, req.Block); err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, map[string]string{"status": "updated", "blocked": fmt.Sprintf("%v", req.Block)})
	return nil
}

// @Summary      Criar Canal (Newsletter)
// @Description  Cria um novo canal público de transmissão do WhatsApp com nome, descrição e avatar opcional.
// @Tags         WhatsApp - Canais
// @Accept       json
// @Produce      json
// @Param        request body CreateChannelRequest true "Dados do Canal"
// @Success      201 {object} domain.NewsletterInfo
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/channels [post]
func (h *WhatsAppHandler) handleCreateChannel(w http.ResponseWriter, r *http.Request) error {
	var req CreateChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}
	if req.SessionID == "" || req.Name == "" {
		return apierrors.NewBadRequest("session_id and name are required")
	}
	var avatar []byte
	if req.AvatarBase64 != "" {
		avatar, _ = base64.StdEncoding.DecodeString(req.AvatarBase64)
	}
	info, err := h.service.CreateNewsletter(r.Context(), req.SessionID, req.Name, req.Description, avatar)
	if err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusCreated, info)
	return nil
}

// @Summary      Listar Canais Inscritos
// @Description  Retorna os canais do WhatsApp que a sessão segue.
// @Tags         WhatsApp - Canais
// @Produce      json
// @Param        session_id query string true "ID da Sessão"
// @Success      200 {array} domain.NewsletterInfo
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/channels [get]
func (h *WhatsAppHandler) handleListChannels(w http.ResponseWriter, r *http.Request) error {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		return apierrors.NewBadRequest("session_id is required")
	}
	channels, err := h.service.GetSubscribedNewsletters(r.Context(), sessionID)
	if err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, channels)
	return nil
}

// @Summary      Seguir Canal
// @Description  Inscreve a sessão para seguir as atualizações de um canal oficial do WhatsApp.
// @Tags         WhatsApp - Canais
// @Accept       json
// @Produce      json
// @Param        jid path string true "JID do Canal"
// @Param        request body FollowChannelRequest true "Dados do Canal"
// @Success      200 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/channels/{jid}/follow [post]
func (h *WhatsAppHandler) handleFollowChannel(w http.ResponseWriter, r *http.Request) error {
	channelJID := chi.URLParam(r, "jid")
	var req FollowChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}
	if req.ChannelJID == "" {
		req.ChannelJID = channelJID
	}
	if req.SessionID == "" || req.ChannelJID == "" {
		return apierrors.NewBadRequest("session_id and channel_jid are required")
	}
	if err := h.service.FollowNewsletter(r.Context(), req.SessionID, req.ChannelJID); err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, map[string]string{"status": "followed"})
	return nil
}

// @Summary      Deixar de Seguir Canal
// @Description  Cancela a inscrição da sessão em um canal do WhatsApp.
// @Tags         WhatsApp - Canais
// @Accept       json
// @Produce      json
// @Param        jid path string true "JID do Canal"
// @Param        request body FollowChannelRequest true "Dados do Canal"
// @Success      200 {object} map[string]string
// @Failure      400 {object} apierrors.AppError
// @Failure      500 {object} apierrors.AppError
// @Security     ApiKeyAuth
// @Security     InternalKeyAuth
// @Router       /api/v1/channels/{jid}/unfollow [post]
func (h *WhatsAppHandler) handleUnfollowChannel(w http.ResponseWriter, r *http.Request) error {
	channelJID := chi.URLParam(r, "jid")
	var req FollowChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return apierrors.NewBadRequest("invalid request body")
	}
	if req.ChannelJID == "" {
		req.ChannelJID = channelJID
	}
	if req.SessionID == "" || req.ChannelJID == "" {
		return apierrors.NewBadRequest("session_id and channel_jid are required")
	}
	if err := h.service.UnfollowNewsletter(r.Context(), req.SessionID, req.ChannelJID); err != nil {
		return apierrors.NewInternalServerError(err.Error())
	}
	httpPlatform.JSON(w, http.StatusOK, map[string]string{"status": "unfollowed"})
	return nil
}
