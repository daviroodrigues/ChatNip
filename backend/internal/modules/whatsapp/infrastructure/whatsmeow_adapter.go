package infrastructure

import (
	"chatnip/internal/modules/whatsapp/domain"
	"context"
	"errors"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type ZerologAdapter struct {
	zLog   zerolog.Logger
	module string
}

func NewWaLogAdapter(zLog zerolog.Logger, module string) waLog.Logger {
	return &ZerologAdapter{
		zLog:   zLog.With().Str("module", module).Logger(),
		module: module,
	}
}

func (l *ZerologAdapter) Errorf(format string, args ...interface{}) {
	l.zLog.Error().Msgf(format, args...)
}

func (l *ZerologAdapter) Warnf(format string, args ...interface{}) {
	l.zLog.Warn().Msgf(format, args...)
}

func (l *ZerologAdapter) Infof(format string, args ...interface{}) {
	l.zLog.Info().Msgf(format, args...)
}

func (l *ZerologAdapter) Debugf(format string, args ...interface{}) {
	l.zLog.Debug().Msgf(format, args...)
}

func (l *ZerologAdapter) Sub(module string) waLog.Logger {
	return NewWaLogAdapter(l.zLog, module)
}

func (w *WhatsMeowAdapter) SetGroupLocked(ctx context.Context, groupJID string, locked bool) error {
	if w.client == nil {
		return errors.New("client not initialized")
	}
	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return err
	}
	return w.client.SetGroupLocked(ctx, jid, locked)
}

func (w *WhatsMeowAdapter) SetGroupAnnounce(ctx context.Context, groupJID string, announce bool) error {
	if w.client == nil {
		return errors.New("client not initialized")
	}
	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return err
	}
	return w.client.SetGroupAnnounce(ctx, jid, announce)
}

func (w *WhatsMeowAdapter) GetGroupInfo(ctx context.Context, groupJID string) (*domain.GroupMetadata, error) {
	if w.client == nil {
		return nil, errors.New("client not initialized")
	}
	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return nil, err
	}
	info, err := w.client.GetGroupInfo(ctx, jid)
	if err != nil {
		return nil, err
	}
	var participants []domain.GroupParticipant
	for _, p := range info.Participants {
		participants = append(participants, domain.GroupParticipant{
			JID:          p.JID.String(),
			PhoneNumber:  p.PhoneNumber.String(),
			IsAdmin:      p.IsAdmin,
			IsSuperAdmin: p.IsSuperAdmin,
			DisplayName:  p.DisplayName,
		})
	}
	return &domain.GroupMetadata{
		JID:              info.JID.String(),
		OwnerJID:         info.OwnerJID.String(),
		Name:             info.GroupName.Name,
		Topic:            info.GroupTopic.Topic,
		TopicSetBy:       info.GroupTopic.TopicSetBy.String(),
		IsLocked:         info.GroupLocked.IsLocked,
		IsAnnounce:       info.GroupAnnounce.IsAnnounce,
		IsEphemeral:      info.GroupEphemeral.IsEphemeral,
		ParticipantCount: len(info.Participants),
		Participants:     participants,
		CreatedAt:        info.GroupCreated,
	}, nil
}

func (w *WhatsMeowAdapter) SetGroupName(ctx context.Context, groupJID string, name string) error {
	if w.client == nil {
		return errors.New("client not initialized")
	}
	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return err
	}
	return w.client.SetGroupName(ctx, jid, name)
}

func (w *WhatsMeowAdapter) SetGroupTopic(ctx context.Context, groupJID string, topic string) error {
	if w.client == nil {
		return errors.New("client not initialized")
	}
	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return err
	}
	return w.client.SetGroupTopic(ctx, jid, "", "", topic)
}

func (w *WhatsMeowAdapter) SetGroupPhoto(ctx context.Context, groupJID string, avatar []byte) (string, error) {
	if w.client == nil {
		return "", errors.New("client not initialized")
	}
	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return "", err
	}
	return w.client.SetGroupPhoto(ctx, jid, avatar)
}

func (w *WhatsMeowAdapter) UpdateGroupParticipants(ctx context.Context, groupJID string, participants []string, action string) ([]domain.GroupParticipant, error) {
	if w.client == nil {
		return nil, errors.New("client not initialized")
	}
	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return nil, err
	}

	var parsedJIDs []types.JID
	for _, p := range participants {
		pJID, err := types.ParseJID(p)
		if err != nil {
			if !strings.Contains(p, "@") {
				pJID, err = types.ParseJID(p + "@s.whatsapp.net")
			}
		}
		if err == nil {
			parsedJIDs = append(parsedJIDs, pJID)
		}
	}

	var waAction whatsmeow.ParticipantChange
	switch strings.ToLower(action) {
	case "add":
		waAction = whatsmeow.ParticipantChangeAdd
	case "remove":
		waAction = whatsmeow.ParticipantChangeRemove
	case "promote":
		waAction = whatsmeow.ParticipantChangePromote
	case "demote":
		waAction = whatsmeow.ParticipantChangeDemote
	default:
		return nil, errors.New("invalid participant action: must be add, remove, promote or demote")
	}

	res, err := w.client.UpdateGroupParticipants(ctx, jid, parsedJIDs, waAction)
	if err != nil {
		return nil, err
	}

	var updated []domain.GroupParticipant
	for _, p := range res {
		updated = append(updated, domain.GroupParticipant{
			JID:          p.JID.String(),
			PhoneNumber:  p.PhoneNumber.String(),
			IsAdmin:      p.IsAdmin,
			IsSuperAdmin: p.IsSuperAdmin,
			DisplayName:  p.DisplayName,
		})
	}
	return updated, nil
}

func (w *WhatsMeowAdapter) LeaveGroup(ctx context.Context, groupJID string) error {
	if w.client == nil {
		return errors.New("client not initialized")
	}
	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return err
	}
	return w.client.LeaveGroup(ctx, jid)
}

func (w *WhatsMeowAdapter) GetGroupInviteLink(ctx context.Context, groupJID string, reset bool) (string, error) {
	if w.client == nil {
		return "", errors.New("client not initialized")
	}
	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return "", err
	}
	return w.client.GetGroupInviteLink(ctx, jid, reset)
}

func (w *WhatsMeowAdapter) GetGroupInfoFromLink(ctx context.Context, code string) (*domain.GroupInviteInfo, error) {
	if w.client == nil {
		return nil, errors.New("client not initialized")
	}
	info, err := w.client.GetGroupInfoFromLink(ctx, code)
	if err != nil {
		return nil, err
	}
	return &domain.GroupInviteInfo{
		JID:              info.JID.String(),
		GroupName:        info.GroupName.Name,
		OwnerJID:         info.OwnerJID.String(),
		ParticipantCount: len(info.Participants),
		CreatedAt:        info.GroupCreated,
	}, nil
}

func (w *WhatsMeowAdapter) JoinGroupWithLink(ctx context.Context, code string) (string, error) {
	if w.client == nil {
		return "", errors.New("client not initialized")
	}
	jid, err := w.client.JoinGroupWithLink(ctx, code)
	if err != nil {
		return "", err
	}
	return jid.String(), nil
}

func (w *WhatsMeowAdapter) JoinGroupWithInvite(ctx context.Context, groupJID, inviterJID, code string, expiration int64) error {
	if w.client == nil {
		return errors.New("client not initialized")
	}
	gJID, err := types.ParseJID(groupJID)
	if err != nil {
		return err
	}
	iJID, err := types.ParseJID(inviterJID)
	if err != nil {
		return err
	}
	return w.client.JoinGroupWithInvite(ctx, gJID, iJID, code, expiration)
}

func (w *WhatsMeowAdapter) IsOnWhatsApp(ctx context.Context, phones []string) ([]domain.IsOnWhatsAppResult, error) {
	if w.client == nil {
		return nil, errors.New("client not initialized")
	}
	resp, err := w.client.IsOnWhatsApp(ctx, phones)
	if err != nil {
		return nil, err
	}
	var results []domain.IsOnWhatsAppResult
	for _, r := range resp {
		results = append(results, domain.IsOnWhatsAppResult{
			Phone: r.Query,
			JID:   r.JID.String(),
			IsIn:  r.IsIn,
		})
	}
	return results, nil
}

func (w *WhatsMeowAdapter) GetUserInfo(ctx context.Context, jids []string) (map[string]domain.UserInfoResult, error) {
	if w.client == nil {
		return nil, errors.New("client not initialized")
	}
	var parsedJIDs []types.JID
	for _, j := range jids {
		if parsed, err := types.ParseJID(j); err == nil {
			parsedJIDs = append(parsedJIDs, parsed)
		}
	}
	infoMap, err := w.client.GetUserInfo(ctx, parsedJIDs)
	if err != nil {
		return nil, err
	}
	result := make(map[string]domain.UserInfoResult)
	for jid, info := range infoMap {
		res := domain.UserInfoResult{
			JID:    jid.String(),
			Status: info.Status,
		}
		if info.VerifiedName != nil && info.VerifiedName.Details != nil {
			res.VerifiedName = info.VerifiedName.Details.GetVerifiedName()
		}
		if pic, err := w.client.GetProfilePictureInfo(ctx, jid, &whatsmeow.GetProfilePictureParams{}); err == nil && pic != nil {
			res.PictureURL = pic.URL
		}
		result[jid.String()] = res
	}
	return result, nil
}

func (w *WhatsMeowAdapter) GetContacts(ctx context.Context) ([]domain.ContactInfo, error) {
	if w.client == nil || w.client.Store == nil || w.client.Store.Contacts == nil {
		return nil, errors.New("contacts store not available")
	}
	contacts, err := w.client.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		return nil, err
	}
	var results []domain.ContactInfo
	for jid, c := range contacts {
		results = append(results, domain.ContactInfo{
			JID:      jid.String(),
			PushName: c.PushName,
			FullName: c.FullName,
		})
	}
	return results, nil
}

func (w *WhatsMeowAdapter) GetBlocklist(ctx context.Context) ([]string, error) {
	if w.client == nil {
		return nil, errors.New("client not initialized")
	}
	bl, err := w.client.GetBlocklist(ctx, "")
	if err != nil {
		return nil, err
	}
	var results []string
	for _, item := range bl.Items {
		jid := item.PN
		if jid.IsEmpty() {
			jid = item.LID
		}
		if !jid.IsEmpty() {
			results = append(results, jid.String())
		}
	}
	return results, nil
}

func (w *WhatsMeowAdapter) UpdateBlocklist(ctx context.Context, jidStr string, block bool) error {
	if w.client == nil {
		return errors.New("client not initialized")
	}
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return err
	}
	action := events.BlocklistChangeActionBlock
	if !block {
		action = events.BlocklistChangeActionUnblock
	}
	_, err = w.client.UpdateBlocklist(ctx, jid, action, "")
	return err
}

func (w *WhatsMeowAdapter) SetChatPin(ctx context.Context, chatJID string, pin bool) error {
	if w.client == nil {
		return errors.New("client not initialized")
	}
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return err
	}
	return w.client.SendAppState(ctx, appstate.BuildPin(jid, pin))
}

func (w *WhatsMeowAdapter) SetChatMute(ctx context.Context, chatJID string, mute bool, duration time.Duration) error {
	if w.client == nil {
		return errors.New("client not initialized")
	}
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return err
	}
	return w.client.SendAppState(ctx, appstate.BuildMute(jid, mute, duration))
}

func (w *WhatsMeowAdapter) SetChatArchive(ctx context.Context, chatJID string, archive bool) error {
	if w.client == nil {
		return errors.New("client not initialized")
	}
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return err
	}
	return w.client.SendAppState(ctx, appstate.BuildArchive(jid, archive, time.Now(), nil))
}

func (w *WhatsMeowAdapter) DeleteChat(ctx context.Context, chatJID string) error {
	if w.client == nil {
		return errors.New("client not initialized")
	}
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return err
	}
	return w.client.SendAppState(ctx, appstate.BuildDeleteChat(jid, time.Now(), nil, true))
}

func (w *WhatsMeowAdapter) CreateNewsletter(ctx context.Context, name, description string, avatar []byte) (*domain.NewsletterInfo, error) {
	if w.client == nil {
		return nil, errors.New("client not initialized")
	}
	meta, err := w.client.CreateNewsletter(ctx, whatsmeow.CreateNewsletterParams{
		Name:        name,
		Description: description,
		Picture:     avatar,
	})
	if err != nil {
		return nil, err
	}
	return &domain.NewsletterInfo{
		JID:         meta.ID.String(),
		Name:        meta.ThreadMeta.Name.Text,
		Description: meta.ThreadMeta.Description.Text,
		InviteCode:  meta.ThreadMeta.InviteCode,
		Subscribers: meta.ThreadMeta.SubscriberCount,
	}, nil
}

func (w *WhatsMeowAdapter) GetSubscribedNewsletters(ctx context.Context) ([]domain.NewsletterInfo, error) {
	if w.client == nil {
		return nil, errors.New("client not initialized")
	}
	list, err := w.client.GetSubscribedNewsletters(ctx)
	if err != nil {
		return nil, err
	}
	var results []domain.NewsletterInfo
	for _, meta := range list {
		results = append(results, domain.NewsletterInfo{
			JID:         meta.ID.String(),
			Name:        meta.ThreadMeta.Name.Text,
			Description: meta.ThreadMeta.Description.Text,
			InviteCode:  meta.ThreadMeta.InviteCode,
			Subscribers: meta.ThreadMeta.SubscriberCount,
		})
	}
	return results, nil
}

func (w *WhatsMeowAdapter) FollowNewsletter(ctx context.Context, jidStr string) error {
	if w.client == nil {
		return errors.New("client not initialized")
	}
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return err
	}
	return w.client.FollowNewsletter(ctx, jid)
}

func (w *WhatsMeowAdapter) UnfollowNewsletter(ctx context.Context, jidStr string) error {
	if w.client == nil {
		return errors.New("client not initialized")
	}
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return err
	}
	return w.client.UnfollowNewsletter(ctx, jid)
}
