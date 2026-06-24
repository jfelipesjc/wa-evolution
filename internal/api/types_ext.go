package api

// Request/response shapes for the extended Evolution-parity surface. Field names
// mirror Evolution's payloads so existing clients are drop-in compatible.

// --- messages ---

type sendPollReq struct {
	Number          string   `json:"number"`
	Name            string   `json:"name"`
	Values          []string `json:"values"`
	SelectableCount int      `json:"selectableCount"`
}

type sendStickerReq struct {
	Number   string `json:"number"`
	Sticker  string `json:"sticker"` // base64 or data URI (webp)
	Mimetype string `json:"mimetype"`
}

type sendAudioReq struct {
	Number   string `json:"number"`
	Audio    string `json:"audio"` // base64 or data URI
	Mimetype string `json:"mimetype"`
}

// --- chat ---

type archiveChatReq struct {
	Chat    string `json:"chat"`
	LastMessage *struct {
		Key messageKey `json:"key"`
	} `json:"lastMessage,omitempty"`
	Archive bool `json:"archive"`
}

type jidQueryReq struct {
	Number string `json:"number"`
}

type updatePrivacyReq struct {
	// Evolution sends one or more settings; we accept a single {name,value} plus a
	// map form for convenience.
	Name     string            `json:"name"`
	Value    string            `json:"value"`
	Settings map[string]string `json:"settings"`
}

type updateBlockStatusReq struct {
	Number string `json:"number"`
	Status string `json:"status"` // "block" | "unblock"
}

type updateProfilePictureReq struct {
	Number  string `json:"number"`  // optional; group jid or self
	Picture string `json:"picture"` // base64 or data URI (jpeg)
}

type profilePictureResp struct {
	WUID       string `json:"wuid"`
	ProfilePic string `json:"profilePictureUrl"`
}

type businessProfileResp struct {
	WUID        string `json:"wuid"`
	Address     string `json:"address,omitempty"`
	Description string `json:"description,omitempty"`
	Website     string `json:"website,omitempty"`
	Email       string `json:"email,omitempty"`
}

type profileResp struct {
	WUID       string `json:"wuid"`
	PictureURL string `json:"picture,omitempty"`
	Status     string `json:"status,omitempty"`
}

type chatRecord struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	UnreadCount int    `json:"unreadCount"`
	Archived    bool   `json:"archived"`
	Pinned      bool   `json:"pinned"`
	Muted       bool   `json:"muted"`
	Timestamp   int64  `json:"conversationTimestamp,omitempty"`
}

type contactRecord struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Notify   string `json:"notify,omitempty"`
	PushName string `json:"pushName,omitempty"`
}

// --- group ---

type acceptInviteReq struct {
	InviteCode string `json:"inviteCode"`
}

type acceptInviteResp struct {
	GroupJID string `json:"groupJid"`
}

type updateGroupSubjectReq struct {
	GroupJID string `json:"groupJid"`
	Subject  string `json:"subject"`
}

type updateGroupDescriptionReq struct {
	GroupJID    string `json:"groupJid"`
	Description string `json:"description"`
}

type updateGroupPictureReq struct {
	GroupJID string `json:"groupJid"`
	Picture  string `json:"picture"` // base64 or data URI (jpeg)
}

type toggleEphemeralReq struct {
	GroupJID   string `json:"groupJid"`
	Expiration int    `json:"expiration"` // seconds (0,86400,604800,7776000)
}

type updateGroupSettingReq struct {
	GroupJID string `json:"groupJid"`
	Action   string `json:"action"` // announcement|not_announcement|locked|unlocked
}

type sendInviteReq struct {
	GroupJID    string   `json:"groupJid"`
	Numbers     []string `json:"numbers"`
	Description  string   `json:"description"`
}

type revokeInviteResp struct {
	InviteCode string `json:"inviteCode"`
	InviteURL  string `json:"inviteUrl"`
}

// --- labels ---

type labelRecord struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

type handleLabelReq struct {
	Number  string `json:"number"`
	LabelID string `json:"labelId"`
	Action  string `json:"action"` // add|remove
}

// --- call ---

type offerCallReq struct {
	Number string `json:"number"`
	IsVideo bool   `json:"isVideo"`
}

type offerCallResp struct {
	CallID string `json:"callId"`
	Status string `json:"status"`
}

// --- business catalog ---

type getCatalogReq struct {
	Number string `json:"number"`
	Limit  int    `json:"limit"`
}

type productRecord struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Price       int64  `json:"price,omitempty"`
	Currency    string `json:"currency,omitempty"`
}

// --- instance presence ---

type setPresenceReq struct {
	Presence string `json:"presence"` // available|unavailable
}

// --- settings / proxy (instance config; stored + echoed) ---

type settingsBody struct {
	RejectCall      bool   `json:"rejectCall"`
	MsgCall         string `json:"msgCall"`
	GroupsIgnore    bool   `json:"groupsIgnore"`
	AlwaysOnline    bool   `json:"alwaysOnline"`
	ReadMessages    bool   `json:"readMessages"`
	ReadStatus      bool   `json:"readStatus"`
	SyncFullHistory bool   `json:"syncFullHistory"`
}

type proxyBody struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	Protocol string `json:"protocol"`
	Username string `json:"username"`
	Password string `json:"password"`
}
