package handler

import (
	"chatnip/internal/kit/apierrors"
	"chatnip/internal/kit/config"
	"chatnip/internal/kit/middleware"
	"chatnip/internal/kit/router"
	"github.com/go-chi/chi/v5"
)

func (h *WhatsAppHandler) RegisterRoutes(r chi.Router, cfg *config.Config, keyValidator middleware.APIKeyValidator) {
	r.Route(router.ApiPath(cfg.APIVersion, ""), func(r chi.Router) {
		r.Use(middleware.ApiKeyAuth(keyValidator, cfg.InternalKey))

		r.Post("/login", apierrors.APIHandler(h.handleLogin).ServeHTTP)
		r.Post("/login/pair-code", apierrors.APIHandler(h.handleLoginPairCode).ServeHTTP)
		r.Post("/logout", apierrors.APIHandler(h.handleLogout).ServeHTTP)
		r.Get("/status", apierrors.APIHandler(h.handleStatus).ServeHTTP)
		r.Get("/sessions", apierrors.APIHandler(h.handleListSessions).ServeHTTP)
		r.Put("/proxy", apierrors.APIHandler(h.handleUpdateProxy).ServeHTTP)
		r.Post("/proxy/validate", apierrors.APIHandler(h.handleValidateProxy).ServeHTTP)

		r.Post("/send", apierrors.APIHandler(h.handleSend).ServeHTTP)
		r.Post("/sendGroupMessage", apierrors.APIHandler(h.handleSend).ServeHTTP)
		r.Post("/messages/edit", apierrors.APIHandler(h.handleEditMessage).ServeHTTP)
		r.Post("/messages/reaction", apierrors.APIHandler(h.handleReactionMessage).ServeHTTP)
		r.Post("/messages/revoke", apierrors.APIHandler(h.handleRevokeMessage).ServeHTTP)
		r.Post("/messages/read", apierrors.APIHandler(h.handleReadMessage).ServeHTTP)

		r.Post("/status/send", apierrors.APIHandler(h.handleSendStatus).ServeHTTP)

		r.Post("/groups", apierrors.APIHandler(h.handleCreateGroup).ServeHTTP)
		r.Get("/groups", apierrors.APIHandler(h.handleListGroups).ServeHTTP)
		r.Get("/groups/{jid}", apierrors.APIHandler(h.handleGetGroupInfo).ServeHTTP)
		r.Put("/groups/{jid}/name", apierrors.APIHandler(h.handleSetGroupName).ServeHTTP)
		r.Put("/groups/{jid}/topic", apierrors.APIHandler(h.handleSetGroupTopic).ServeHTTP)
		r.Put("/groups/{jid}/photo", apierrors.APIHandler(h.handleSetGroupPhoto).ServeHTTP)
		r.Delete("/groups/{jid}/photo", apierrors.APIHandler(h.handleDeleteGroupPhoto).ServeHTTP)
		r.Put("/groups/{jid}/settings/info", apierrors.APIHandler(h.handleSetGroupInfoSetting).ServeHTTP)
		r.Put("/groups/settings/messages", apierrors.APIHandler(h.handleSetGroupMessageSetting).ServeHTTP)
		r.Post("/groups/{jid}/participants", apierrors.APIHandler(h.handleUpdateGroupParticipants).ServeHTTP)
		r.Post("/groups/{jid}/leave", apierrors.APIHandler(h.handleLeaveGroup).ServeHTTP)
		r.Get("/groups/{jid}/invite", apierrors.APIHandler(h.handleGetGroupInviteLink).ServeHTTP)
		r.Post("/groups/{jid}/invite/revoke", apierrors.APIHandler(h.handleRevokeGroupInviteLink).ServeHTTP)
		r.Get("/groups/invites/{code}", apierrors.APIHandler(h.handleGetGroupInfoFromInvite).ServeHTTP)
		r.Post("/groups/join", apierrors.APIHandler(h.handleJoinGroup).ServeHTTP)
		r.Post("/groups/join-invite", apierrors.APIHandler(h.handleJoinGroupInviteMessage).ServeHTTP)

		r.Post("/contacts/check", apierrors.APIHandler(h.handleCheckNumbers).ServeHTTP)
		r.Post("/contacts/resolve", apierrors.APIHandler(h.handleResolveContact).ServeHTTP)
		r.Get("/contacts", apierrors.APIHandler(h.handleListContacts).ServeHTTP)
		r.Get("/contacts/{jid}/info", apierrors.APIHandler(h.handleGetUserInfo).ServeHTTP)
		r.Put("/chats/{jid}/pin", apierrors.APIHandler(h.handleSetChatPin).ServeHTTP)
		r.Put("/chats/{jid}/mute", apierrors.APIHandler(h.handleSetChatMute).ServeHTTP)
		r.Put("/chats/{jid}/archive", apierrors.APIHandler(h.handleSetChatArchive).ServeHTTP)
		r.Delete("/chats/{jid}", apierrors.APIHandler(h.handleDeleteChat).ServeHTTP)
		r.Get("/blocklist", apierrors.APIHandler(h.handleGetBlocklist).ServeHTTP)
		r.Post("/blocklist", apierrors.APIHandler(h.handleUpdateBlocklist).ServeHTTP)

		r.Post("/channels", apierrors.APIHandler(h.handleCreateChannel).ServeHTTP)
		r.Get("/channels", apierrors.APIHandler(h.handleListChannels).ServeHTTP)
		r.Post("/channels/{jid}/send", apierrors.APIHandler(h.handleSendChannelMessage).ServeHTTP)
		r.Post("/channels/{jid}/follow", apierrors.APIHandler(h.handleFollowChannel).ServeHTTP)
		r.Post("/channels/{jid}/unfollow", apierrors.APIHandler(h.handleUnfollowChannel).ServeHTTP)
	})
}
