package api

// This file defines the request/response DTOs the HTTP layer marshals, shaped to
// mirror the Evolution API v2 contract the user's Chatwoot/workers consume. The
// field names and JSON tags intentionally match Evolution so existing clients
// need no changes.

// --- instance lifecycle ---

// createInstanceReq is the body of POST /instance/create.
type createInstanceReq struct {
	InstanceName string `json:"instanceName"`
	WebhookURL   string `json:"webhookUrl,omitempty"`
}

// instanceInfo is the per-instance descriptor returned by create/fetch.
type instanceInfo struct {
	InstanceName     string `json:"instanceName"`
	ConnectionStatus string `json:"connectionStatus"`
}

// createInstanceResp is the body of POST /instance/create.
type createInstanceResp struct {
	Instance instanceInfo `json:"instance"`
	Hash     string       `json:"hash,omitempty"`
}

// connectResp is the body of GET /instance/connect/{instance}. Code is the raw
// QR string (base64 PNG); Base64 is the data-URI form Evolution clients render.
type connectResp struct {
	Code        string `json:"code"`
	Base64      string `json:"base64,omitempty"`
	PairingCode string `json:"pairingCode,omitempty"`
	Count       int    `json:"count,omitempty"`
	Instance    string `json:"instance,omitempty"`
	ConnStatus  string `json:"connectionStatus,omitempty"`
}

// statusResp is a generic {status:...} ack (logout/delete/etc.).
type statusResp struct {
	Status string `json:"status"`
}

// --- webhook config ---

type setWebhookReq struct {
	URL string `json:"url"`
	// Evolution nests under {webhook:{url}} in some versions; accept both.
	Webhook *struct {
		URL string `json:"url"`
	} `json:"webhook,omitempty"`
}

type setWebhookResp struct {
	Webhook struct {
		URL string `json:"url"`
	} `json:"webhook"`
}

// findWebhookResp is the body of GET /webhook/find/{instance}. It reports the
// persisted webhook URL for the instance (empty if none configured). enabled
// mirrors Evolution's shape (true when a URL is set).
type findWebhookResp struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}

// --- presence / typing ---

// sendPresenceReq is the body of POST /chat/sendPresence/{instance}. presence is
// one of composing|paused|available|unavailable (Evolution/Baileys vocabulary).
// number is required for composing/paused (per-chat typing); for the global
// available/unavailable it is optional.
type sendPresenceReq struct {
	Number   string `json:"number,omitempty"`
	Presence string `json:"presence"`
}

// --- read receipts ---

// readKey is one {remoteJid,id} entry of markMessageAsRead.
type readKey struct {
	RemoteJID string `json:"remoteJid"`
	ID        string `json:"id"`
}

// markReadReq is the body of POST /message/markMessageAsRead/{instance}. It
// accepts Evolution's readMessages:[{remoteJid,id}] form and a convenience
// {number, ids:[...]} form.
type markReadReq struct {
	ReadMessages []readKey `json:"readMessages,omitempty"`
	Number       string    `json:"number,omitempty"`
	IDs          []string  `json:"ids,omitempty"`
}

// --- group management ---

// groupCreateReq is the body of POST /group/create/{instance}.
type groupCreateReq struct {
	Subject      string   `json:"subject"`
	Participants []string `json:"participants"`
}

// updateParticipantReq is the body of POST /group/updateParticipant/{instance}.
// action is one of add|remove|promote|demote.
type updateParticipantReq struct {
	GroupJID     string   `json:"groupJid"`
	Action       string   `json:"action"`
	Participants []string `json:"participants"`
}

// participantResult is one row of an updateParticipant response.
type participantResult struct {
	JID    string `json:"jid"`
	Status string `json:"status"`
}

// inviteCodeResp is the body of GET /group/inviteCode/{instance}.
type inviteCodeResp struct {
	InviteCode string `json:"inviteCode"`
	InviteURL  string `json:"inviteUrl"`
}

// leaveGroupReq is the body of POST /group/leave/{instance}.
type leaveGroupReq struct {
	GroupJID string `json:"groupJid"`
}

// --- messages ---

type sendTextReq struct {
	Number string `json:"number"`
	Text   string `json:"text"`
}

type sendMediaReq struct {
	Number    string `json:"number"`
	MediaType string `json:"mediatype"` // image|video|audio|document
	Media     string `json:"media"`     // base64 (data-URI or raw) — URLs not fetched offline
	Caption   string `json:"caption,omitempty"`
	FileName  string `json:"fileName,omitempty"`
	Mimetype  string `json:"mimetype,omitempty"`
}

// messageKey mirrors Evolution's key object.
type messageKey struct {
	RemoteJID string `json:"remoteJid"`
	FromMe    bool   `json:"fromMe"`
	ID        string `json:"id"`
}

type sendReactionReq struct {
	Key      messageKey `json:"key"`
	Reaction string     `json:"reaction"`
}

// sendResp is the Evolution send acknowledgement.
type sendResp struct {
	Key              messageKey `json:"key"`
	Status           string     `json:"status"`
	MessageTimestamp int64      `json:"messageTimestamp,omitempty"`
}

// --- chat queries ---

type findMessagesReq struct {
	Where struct {
		Key struct {
			RemoteJID string `json:"remoteJid"`
		} `json:"key"`
	} `json:"where"`
	Limit int `json:"limit,omitempty"`
}

// messageRecord is one row of findMessages, shaped like Evolution's stored
// message record (a subset).
type messageRecord struct {
	Key              messageKey `json:"key"`
	MessageType      string     `json:"messageType"`
	MessageTimestamp int64      `json:"messageTimestamp"`
	PushName         string     `json:"pushName,omitempty"`
	Message          struct {
		Conversation string `json:"conversation,omitempty"`
	} `json:"message"`
}

type findMessagesResp struct {
	Messages struct {
		Records []messageRecord `json:"records"`
	} `json:"messages"`
}

type whatsappNumbersReq struct {
	Numbers []string `json:"numbers"`
}

// numberStatus is one result of /chat/whatsappNumbers.
type numberStatus struct {
	Exists bool   `json:"exists"`
	JID    string `json:"jid"`
	Number string `json:"number"`
}

// --- groups ---

// groupRecord is the metadata of one group (Evolution group shape subset).
type groupRecord struct {
	ID           string             `json:"id"`
	Subject      string             `json:"subject"`
	Owner        string             `json:"owner,omitempty"`
	Desc         string             `json:"desc,omitempty"`
	Creation     int64              `json:"creation,omitempty"`
	Participants []groupParticipant `json:"participants,omitempty"`
}

type groupParticipant struct {
	ID    string `json:"id"`
	Admin string `json:"admin,omitempty"`
}

// --- webhook payload (outbound to Chatwoot/worker) ---

// webhookEnvelope is the Evolution-shaped body POSTed to a configured webhookUrl.
type webhookEnvelope struct {
	Event       string      `json:"event"`
	Instance    string      `json:"instance"`
	Data        interface{} `json:"data"`
	DateTime    string      `json:"date_time"`
	Destination string      `json:"destination,omitempty"`
}
