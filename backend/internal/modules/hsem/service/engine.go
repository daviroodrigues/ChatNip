package service

import (
	"chatnip/internal/modules/hsem/domain"
	"time"
)

type HSEMEngine struct {
	math *MathUtils
}

func NewHSEMEngine(math *MathUtils) *HSEMEngine {
	return &HSEMEngine{
		math: math,
	}
}

func (e *HSEMEngine) Math() *MathUtils {
	return e.math
}

func (e *HSEMEngine) GenerateFingerprint(input domain.SimulationInput) domain.FingerprintPlan {
	plan := domain.FingerprintPlan{
		Steps: []domain.ActionStep{},
	}

	personaConfig, exists := domain.PersonaMultipliers[input.Persona]
	if !exists {
		personaConfig = domain.PersonaMultipliers[domain.PersonaStandard]
	}

	currentTimeOffset := time.Duration(0)

	networkLag := e.math.UniformDuration(domain.MinNetworkLatency, domain.MaxNetworkLatency)
	currentTimeOffset += networkLag

	baseReadingTime := float64(input.InputTextLength) * float64(domain.AvgReadingSpeedMs)
	baseReadingTime *= personaConfig.TimeMultiplier

	readingTime := e.math.GaussianDuration(baseReadingTime, domain.ReadingVariance)
	if readingTime < 1*time.Second {
		readingTime = 1 * time.Second
	}

	plan.Steps = append(plan.Steps, domain.ActionStep{
		Type:      domain.StepAction,
		Timestamp: currentTimeOffset + domain.MarkAsReadDelay,
		Metadata:  "mark_as_read",
	})

	currentTimeOffset += readingTime

	probDeadTime := domain.DeadTimeProb
	if input.CurrentFatigue > 0.7 {
		probDeadTime += 0.2
	}
	if input.Persona == domain.PersonaDistracted {
		probDeadTime += 0.15
	}

	if e.math.ShouldHappen(probDeadTime) {
		deadTime := e.math.ExponentialDuration(domain.DeadTimeMin, domain.DeadTimeMax)
		if input.Persona == domain.PersonaDistracted {
			deadTime = time.Duration(float64(deadTime) * 1.5)
		}
		currentTimeOffset += deadTime
	}

	switch input.MessageType {
	case domain.MsgTypeAudioPTT:
		e.planAudioPTT(input, &plan, &currentTimeOffset)
	case domain.MsgTypeImage, domain.MsgTypeVideo:
		e.planVisualMedia(input, &plan, &currentTimeOffset)
	case domain.MsgTypeDocument:
		e.planDocument(&plan, &currentTimeOffset)
	case domain.MsgTypeSticker:
		e.planSticker(&plan, &currentTimeOffset)
	default:
		e.planText(input, &plan, &currentTimeOffset, personaConfig)
	}

	cooldown := domain.CooldownStandard
	if len(input.OutputText) < domain.SmallTextThreshold {
		cooldown = domain.SmallTextCooldown
	} else if input.ConsecutiveMsgs >= domain.SpamMsgThreshold {
		cooldown = domain.SpamCooldown
	}

	cooldownMs := float64(cooldown.Milliseconds()) / personaConfig.ImpatienceFactor
	cooldownVar := e.math.GaussianDuration(cooldownMs, 0.5)
	currentTimeOffset += cooldownVar

	plan.TotalDuration = currentTimeOffset
	return plan
}

func (e *HSEMEngine) planText(input domain.SimulationInput, plan *domain.FingerprintPlan, offset *time.Duration, persona struct {
	TimeMultiplier   float64
	ErrorProb        float64
	ImpatienceFactor float64
}) {
	preAction := e.math.UniformDuration(domain.MinPreAction, domain.MaxPreAction)
	*offset += preAction

	baseTypingTimeMs := float64(len(input.OutputText)) * float64(domain.AvgTypingSpeedMs)
	envMultiplier := e.calculateEnvironmentMultipliers(input)
	totalMultiplier := envMultiplier * persona.TimeMultiplier
	finalTypingDuration := e.math.GaussianDuration(baseTypingTimeMs*totalMultiplier, domain.TypingVariance)

	const LargeTextThreshold = 200
	if len(input.OutputText) >= LargeTextThreshold {
		targetCeilingMs := 35000.0
		minFloorDuration := 15 * time.Second

		if input.Persona == domain.PersonaRushed {
			targetCeilingMs = 12000.0
			minFloorDuration = 4 * time.Second
		}

		const CeilingVariance = 0.25
		softCap := e.math.GaussianDuration(targetCeilingMs, CeilingVariance)
		if softCap < minFloorDuration {
			softCap = minFloorDuration
		}

		if finalTypingDuration > softCap {
			finalTypingDuration = softCap
		}
	}

	if finalTypingDuration > 3*time.Second {
		remainingTime := finalTypingDuration
		for remainingTime > 0 {
			burstDuration := e.math.GaussianDuration(float64(domain.BurstDurationAvg.Milliseconds()), 0.2)
			if burstDuration > remainingTime {
				burstDuration = remainingTime
			}

			plan.Steps = append(plan.Steps, domain.ActionStep{
				Type:      domain.StepStateChange,
				Timestamp: *offset,
				Metadata:  "composing",
			})
			*offset += burstDuration
			remainingTime -= burstDuration

			if remainingTime <= 100*time.Millisecond {
				break
			}

			pauseDuration := e.math.UniformDuration(domain.BurstPauseMin, domain.BurstPauseMax)
			if e.math.ShouldHappen(persona.ErrorProb) {
				pauseDuration += 800 * time.Millisecond
			}

			plan.Steps = append(plan.Steps, domain.ActionStep{
				Type:      domain.StepStateChange,
				Timestamp: *offset,
				Metadata:  "paused",
			})
			*offset += pauseDuration
		}
	} else {
		plan.Steps = append(plan.Steps, domain.ActionStep{
			Type:      domain.StepStateChange,
			Timestamp: *offset,
			Metadata:  "composing",
		})
		*offset += finalTypingDuration
	}

	e.finalizeAction(plan, offset)
}

func (e *HSEMEngine) planAudioPTT(input domain.SimulationInput, plan *domain.FingerprintPlan, offset *time.Duration) {
	*offset += e.math.UniformDuration(domain.MinPreAction, domain.MaxPreAction)

	plan.Steps = append(plan.Steps, domain.ActionStep{
		Type:      domain.StepStateChange,
		Timestamp: *offset,
		Metadata:  "recording",
	})

	humanMargin := e.math.UniformDuration(500*time.Millisecond, 1000*time.Millisecond)
	recordingTime := input.MediaDuration + humanMargin
	*offset += recordingTime

	plan.Steps = append(plan.Steps, domain.ActionStep{
		Type:      domain.StepStateChange,
		Timestamp: *offset,
		Metadata:  "paused",
	})
	*offset += 200 * time.Millisecond

	plan.Steps = append(plan.Steps, domain.ActionStep{
		Type:      domain.StepAction,
		Timestamp: *offset,
		Metadata:  "send_message",
	})
}

func (e *HSEMEngine) planVisualMedia(input domain.SimulationInput, plan *domain.FingerprintPlan, offset *time.Duration) {
	plan.Steps = append(plan.Steps, domain.ActionStep{
		Type:      domain.StepStateChange,
		Timestamp: *offset,
		Metadata:  "composing",
	})

	searchTime := e.math.UniformDuration(2*time.Second, 5*time.Second)
	*offset += searchTime

	if input.FileSizeKB > 0 {
		uploadMs := float64(input.FileSizeKB) / 0.5
		*offset += time.Duration(uploadMs) * time.Millisecond
	} else {
		*offset += 1500 * time.Millisecond
	}

	plan.Steps = append(plan.Steps, domain.ActionStep{
		Type:      domain.StepStateChange,
		Timestamp: *offset,
		Metadata:  "paused",
	})
	plan.Steps = append(plan.Steps, domain.ActionStep{
		Type:      domain.StepAction,
		Timestamp: *offset,
		Metadata:  "send_message",
	})
}

func (e *HSEMEngine) planDocument(plan *domain.FingerprintPlan, offset *time.Duration) {
	plan.Steps = append(plan.Steps, domain.ActionStep{
		Type:      domain.StepStateChange,
		Timestamp: *offset,
		Metadata:  "composing",
	})

	searchTime := e.math.GaussianDuration(3000, 0.2)
	*offset += searchTime

	plan.Steps = append(plan.Steps, domain.ActionStep{
		Type:      domain.StepStateChange,
		Timestamp: *offset,
		Metadata:  "paused",
	})
	plan.Steps = append(plan.Steps, domain.ActionStep{
		Type:      domain.StepAction,
		Timestamp: *offset,
		Metadata:  "send_message",
	})
}

func (e *HSEMEngine) planSticker(plan *domain.FingerprintPlan, offset *time.Duration) {
	plan.Steps = append(plan.Steps, domain.ActionStep{
		Type:      domain.StepStateChange,
		Timestamp: *offset,
		Metadata:  "composing",
	})

	isFavorite := e.math.ShouldHappen(0.7)
	var selectionTime time.Duration
	if isFavorite {
		selectionTime = e.math.UniformDuration(400*time.Millisecond, 800*time.Millisecond)
	} else {
		selectionTime = e.math.UniformDuration(2000*time.Millisecond, 4000*time.Millisecond)
	}
	*offset += selectionTime

	plan.Steps = append(plan.Steps, domain.ActionStep{
		Type:      domain.StepAction,
		Timestamp: *offset,
		Metadata:  "send_message",
	})
}

func (e *HSEMEngine) finalizeAction(plan *domain.FingerprintPlan, offset *time.Duration) {
	postAction := e.math.UniformDuration(domain.MinPostAction, domain.MaxPostAction)

	plan.Steps = append(plan.Steps, domain.ActionStep{
		Type:      domain.StepStateChange,
		Timestamp: *offset,
		Metadata:  "paused",
	})
	*offset += postAction

	plan.Steps = append(plan.Steps, domain.ActionStep{
		Type:      domain.StepAction,
		Timestamp: *offset,
		Metadata:  "send_message",
	})
}

func (e *HSEMEngine) calculateEnvironmentMultipliers(input domain.SimulationInput) float64 {
	mTotal := 1.0
	hour := input.CurrentTime.Hour()

	switch {
	case hour >= 0 && hour < 6:
		mTotal *= 1.5
	case hour >= 6 && hour < 9:
		mTotal *= 1.1
	case hour >= 9 && hour < 12:
		mTotal *= 0.9
	case hour >= 12 && hour < 14:
		mTotal *= 1.3
	case hour >= 14 && hour < 18:
		mTotal *= 1.0
	case hour >= 18:
		mTotal *= 1.1
	}

	if len(input.OutputText) < 10 {
		mTotal *= 0.8
	}

	if input.CurrentFatigue > 0.7 {
		mTotal *= 1.2
	}

	return mTotal
}

func (e *HSEMEngine) CalculateInteractionDelay() time.Duration {
	baseDelay := 1800.0
	return e.math.GaussianDuration(baseDelay, 0.3)
}
