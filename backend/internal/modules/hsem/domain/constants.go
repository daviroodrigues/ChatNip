package domain

import "time"

var PersonaMultipliers = map[PersonaType]struct {
	TimeMultiplier   float64
	ErrorProb        float64
	ImpatienceFactor float64
}{
	PersonaStandard:   {1.0, 0.05, 1.0},
	PersonaRushed:     {0.8, 0.12, 1.5},
	PersonaMeticulous: {1.25, 0.01, 0.5},
	PersonaDistracted: {1.1, 0.06, 2.0},
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
)
