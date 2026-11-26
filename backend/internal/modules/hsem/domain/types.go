package domain

import "time"

type MessageType string

const (
	MsgTypeText     MessageType = "text"
	MsgTypeAudioPTT MessageType = "audio_ptt"
	MsgTypeImage    MessageType = "image"
	MsgTypeVideo    MessageType = "video"
	MsgTypeDocument MessageType = "document"
	MsgTypeSticker  MessageType = "sticker"
)

type SimulationInput struct {
	Persona              PersonaType
	MessageType          MessageType
	InputTextLength      int
	OutputText           string
	MediaDuration        time.Duration
	FileSizeKB           int
	CurrentTime          time.Time
	CurrentFatigue       float64
	ConsecutiveMsgs      int
	PersonalSpeedFactor  float64
	BandwidthKBps        float64
	CircadianOffset      time.Duration
	InSleepWindow        bool
	ConversationWarmth   float64
	WasOffline           bool
	SkipMarkAsRead       bool
	BatchReadingOverhead time.Duration
}

type StepType string

const (
	StepWait          StepType    = "WAIT"
	StepAction        StepType    = "ACTION"
	StepStateChange   StepType    = "STATE"
	PersonaStandard   PersonaType = "PADRAO"
	PersonaRushed     PersonaType = "APRESSADO"
	PersonaMeticulous PersonaType = "METICULOSO"
	PersonaDistracted PersonaType = "DISTRAIDO"
)

type ActionStep struct {
	Type      StepType
	Duration  time.Duration
	Timestamp time.Duration
	Metadata  string
}

type FingerprintPlan struct {
	TotalDuration time.Duration
	Steps         []ActionStep
}

type PersonaType string
