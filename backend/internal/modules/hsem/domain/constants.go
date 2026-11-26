package domain

import "time"

type PersonaConfig struct {
	TimeMultiplier   float64
	ErrorProb        float64
	ImpatienceFactor float64
}

var PersonaMultipliers = map[PersonaType]PersonaConfig{
	PersonaStandard:   {TimeMultiplier: 1.0, ErrorProb: 0.060, ImpatienceFactor: 1.0},
	PersonaRushed:     {TimeMultiplier: 0.8, ErrorProb: 0.077, ImpatienceFactor: 1.5},
	PersonaMeticulous: {TimeMultiplier: 1.25, ErrorProb: 0.045, ImpatienceFactor: 0.5},
	PersonaDistracted: {TimeMultiplier: 1.1, ErrorProb: 0.110, ImpatienceFactor: 2.0},
}

const (
	AvgTypingSpeedMs   = 180
	TypingVariance     = 0.35
	AvgReadingSpeedMs  = 45
	ReadingVariance    = 0.15
	MinNetworkLatency  = 100 * time.Millisecond
	MaxNetworkLatency  = 800 * time.Millisecond
	DeadTimeMin        = 3 * time.Second
	DeadTimeMax        = 12 * time.Second
	DeadTimeProb       = 0.15
	MinPreAction       = 200 * time.Millisecond
	MaxPreAction       = 600 * time.Millisecond
	MinPostAction      = 150 * time.Millisecond
	MaxPostAction      = 500 * time.Millisecond
	CooldownStandard   = 1500 * time.Millisecond
	BurstDurationAvg   = 2500 * time.Millisecond
	BurstPauseMin      = 200 * time.Millisecond
	BurstPauseMax      = 600 * time.Millisecond
	SmallTextThreshold = 15
	SmallTextCooldown  = 600 * time.Millisecond
	SpamMsgThreshold   = 4
	SpamCooldown       = 8000 * time.Millisecond
	MarkAsReadDelay    = 200 * time.Millisecond

	SleepInitialLatencyMin = 3 * time.Second
	SleepInitialLatencyMax = 15 * time.Second
	SleepDeadTimeBonus     = 0.25
	PeekDelayProb          = 0.15
	PeekDelayMin           = 2 * time.Second
	PeekDelayMax           = 6 * time.Second

	LongTextSmoothThreshold = 160
	CopyPasteThreshold      = 350
	CopyPasteProb           = 0.35
	CopyPasteDurationMin    = 1500 * time.Millisecond
	CopyPasteDurationMax    = 3500 * time.Millisecond
	DraftDeleteMinChars     = 40
	DraftDeleteFatigue      = 0.6
	DraftDeleteProb         = 0.08
	DraftComposingMin       = 1200 * time.Millisecond
	DraftComposingMax       = 2500 * time.Millisecond
	DraftPauseMin           = 1500 * time.Millisecond
	DraftPauseMax           = 3000 * time.Millisecond

	DefaultBandwidthKBps = 8192.0
	MinBandwidthKBps     = 2048.0
	MaxBandwidthKBps     = 35840.0
	UploadRTTMin         = 250 * time.Millisecond
	UploadRTTMax         = 700 * time.Millisecond
	AudioMarginMin       = 400 * time.Millisecond
	AudioMarginMax       = 1200 * time.Millisecond

	MaxContinuousComposingMin = 12 * time.Second
	MaxContinuousComposingMax = 15 * time.Second
	ForcedMicroPauseMin       = 350 * time.Millisecond
	ForcedMicroPauseMax       = 800 * time.Millisecond
)
