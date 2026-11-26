package service

import (
	"chatnip/internal/modules/hsem/domain"
	"math"
	"time"
)

type HSEMEngine struct {
	math *MathUtils
}

func NewHSEMEngine(math *MathUtils) *HSEMEngine {
	if math == nil {
		math = NewMathUtils()
	}
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

	warmth := clampFloat(input.ConversationWarmth, 0.0, 1.0)
	currentTimeOffset := time.Duration(0)

	networkLag := e.math.UniformDuration(domain.MinNetworkLatency, domain.MaxNetworkLatency)
	currentTimeOffset += networkLag

	if input.InSleepWindow {
		currentTimeOffset += e.math.UniformDuration(domain.SleepInitialLatencyMin, domain.SleepInitialLatencyMax)
	}

	warmthReadFactor := 1.0 - (0.15 * warmth)
	baseReadingTime := float64(input.InputTextLength) * float64(domain.AvgReadingSpeedMs)
	baseReadingTime *= personaConfig.TimeMultiplier * warmthReadFactor

	readingTime := e.math.GaussianDuration(baseReadingTime, domain.ReadingVariance)
	if readingTime < 1*time.Second {
		readingTime = 1 * time.Second
	}
	readingTime += input.BatchReadingOverhead

	if !input.SkipMarkAsRead {
		markDelay := domain.MarkAsReadDelay + e.math.UniformDuration(0, 180*time.Millisecond)
		if input.WasOffline && e.math.ShouldHappen(domain.PeekDelayProb) {
			markDelay += e.math.UniformDuration(domain.PeekDelayMin, domain.PeekDelayMax)
		}
		plan.Steps = append(plan.Steps, domain.ActionStep{
			Type:      domain.StepAction,
			Timestamp: currentTimeOffset + markDelay,
			Metadata:  "mark_as_read",
		})
	}

	currentTimeOffset += readingTime

	probDeadTime := domain.DeadTimeProb
	if input.CurrentFatigue > 0.7 {
		probDeadTime += 0.20
	}
	if input.Persona == domain.PersonaDistracted {
		probDeadTime += 0.15
	}
	if input.InSleepWindow {
		probDeadTime += domain.SleepDeadTimeBonus
	}
	probDeadTime *= (1.0 - 0.5*warmth)

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
		e.planDocument(input, &plan, &currentTimeOffset)
	case domain.MsgTypeSticker:
		e.planSticker(&plan, &currentTimeOffset)
	default:
		e.planText(input, &plan, &currentTimeOffset, personaConfig)
	}

	cooldown := domain.CooldownStandard
	if len([]rune(input.OutputText)) < domain.SmallTextThreshold {
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

func (e *HSEMEngine) GenerateControlPlan(action string, input domain.SimulationInput) domain.FingerprintPlan {
	warmth := clampFloat(input.ConversationWarmth, 0.0, 1.0)
	var actionDelay time.Duration

	switch action {
	case "reaction", "react":
		readMs := 400.0 + (1.0-warmth)*500.0
		readDelay := e.math.GaussianDuration(readMs, 0.25)
		pickerDelay := e.math.UniformDuration(900*time.Millisecond, 2800*time.Millisecond)
		actionDelay = readDelay + pickerDelay
	case "revoke", "delete":
		actionDelay = e.math.UniformDuration(1500*time.Millisecond, 4000*time.Millisecond)
	case "read", "mark_read":
		actionDelay = e.math.UniformDuration(600*time.Millisecond, 2200*time.Millisecond)
	default:
		actionDelay = e.math.UniformDuration(120*time.Millisecond, 450*time.Millisecond)
	}

	if input.InSleepWindow {
		actionDelay += e.math.UniformDuration(500*time.Millisecond, 2000*time.Millisecond)
	}

	postMargin := e.math.UniformDuration(80*time.Millisecond, 250*time.Millisecond)
	return domain.FingerprintPlan{
		TotalDuration: actionDelay + postMargin,
		Steps: []domain.ActionStep{
			{
				Type:      domain.StepAction,
				Timestamp: actionDelay,
				Metadata:  "send_message",
			},
		},
	}
}

func (e *HSEMEngine) planText(input domain.SimulationInput, plan *domain.FingerprintPlan, offset *time.Duration, persona domain.PersonaConfig) {
	profile := domain.GetBiometricProfile(input.Persona)
	speedFactor := input.PersonalSpeedFactor
	if speedFactor <= 0 {
		speedFactor = 1.0
	}

	ttfk := e.math.LogNormalDuration(profile.TTFKMedianMs*speedFactor, profile.TTFKSigma, 250*time.Millisecond, 6*time.Second)
	*offset += ttfk

	runes := []rune(input.OutputText)
	runeCount := len(runes)

	if runeCount > domain.CopyPasteThreshold && e.math.ShouldHappen(domain.CopyPasteProb) {
		pasteDuration := e.math.UniformDuration(domain.CopyPasteDurationMin, domain.CopyPasteDurationMax)
		plan.Steps = append(plan.Steps, domain.ActionStep{
			Type:      domain.StepStateChange,
			Timestamp: *offset,
			Metadata:  "composing",
		})
		*offset += pasteDuration
		e.finalizeAction(plan, offset)
		return
	}

	if (runeCount > domain.DraftDeleteMinChars || input.CurrentFatigue > domain.DraftDeleteFatigue) &&
		e.math.ShouldHappen(domain.DraftDeleteProb) {
		draftDur := e.math.UniformDuration(domain.DraftComposingMin, domain.DraftComposingMax)
		pauseDur := e.math.UniformDuration(domain.DraftPauseMin, domain.DraftPauseMax)

		plan.Steps = append(plan.Steps, domain.ActionStep{
			Type:      domain.StepStateChange,
			Timestamp: *offset,
			Metadata:  "composing",
		})
		*offset += draftDur

		plan.Steps = append(plan.Steps, domain.ActionStep{
			Type:      domain.StepStateChange,
			Timestamp: *offset,
			Metadata:  "paused",
		})
		*offset += pauseDur
	}

	segments, totalBaseMs := e.computeEmpiricalSegments(runes, profile)
	smoothedBaseMs := smoothLongTextMs(totalBaseMs, runeCount, profile)

	envMultiplier := e.calculateEnvironmentMultipliers(input)
	targetMs := smoothedBaseMs * envMultiplier * speedFactor
	if targetMs < 250 {
		targetMs = 250
	}

	finalTypingDuration := e.math.LogNormalDuration(targetMs, profile.IKISigma*0.5, 250*time.Millisecond, 65*time.Second)

	if finalTypingDuration > 3*time.Second && len(segments) > 1 {
		totalSegWeight := 0.0
		for _, w := range segments {
			totalSegWeight += w
		}
		if totalSegWeight <= 0 {
			totalSegWeight = 1.0
		}

		for idx, segWeight := range segments {
			segDuration := time.Duration(float64(finalTypingDuration) * (segWeight / totalSegWeight))
			if segDuration < 150*time.Millisecond {
				segDuration = 150 * time.Millisecond
			}

			plan.Steps = append(plan.Steps, domain.ActionStep{
				Type:      domain.StepStateChange,
				Timestamp: *offset,
				Metadata:  "composing",
			})
			*offset += segDuration

			if idx == len(segments)-1 {
				break
			}

			pauseDuration := e.math.UniformDuration(domain.BurstPauseMin, domain.BurstPauseMax)
			if e.math.ShouldHappen(profile.BackspaceRate) {
				pauseDuration += e.math.LogNormalDuration(profile.CorrectionDurationMs, 0.35, 180*time.Millisecond, 2500*time.Millisecond)
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

func (e *HSEMEngine) computeEmpiricalSegments(runes []rune, profile domain.BiometricProfile) ([]float64, float64) {
	if len(runes) == 0 {
		return []float64{profile.BaseIKIMs}, profile.BaseIKIMs
	}

	var segments []float64
	currentSegMs := 0.0
	totalMs := 0.0
	prevCat := domain.CatSpace

	const maxChunkMs = 2800.0

	for i, r := range runes {
		cat := domain.ClassifyRune(r)
		var charMs float64
		if i == 0 {
			charMs = profile.BaseIKIMs
		} else {
			charMs = profile.TransitionCostMs(prevCat, cat)
		}
		charMs += profile.BackspaceRate * (profile.CorrectionDurationMs * 0.35)

		currentSegMs += charMs
		totalMs += charMs
		prevCat = cat

		isDelimiter := r == '.' || r == '?' || r == '!' || r == '\n'
		if (isDelimiter && currentSegMs >= 700.0) || currentSegMs >= maxChunkMs {
			segments = append(segments, currentSegMs)
			currentSegMs = 0
		}
	}

	if currentSegMs > 0 {
		if len(segments) > 0 && currentSegMs < 400.0 {
			segments[len(segments)-1] += currentSegMs
		} else {
			segments = append(segments, currentSegMs)
		}
	}

	return segments, totalMs
}

func smoothLongTextMs(rawMs float64, runeCount int, profile domain.BiometricProfile) float64 {
	if runeCount <= domain.LongTextSmoothThreshold || rawMs <= 0 {
		return rawMs
	}
	avgPerRune := rawMs / float64(runeCount)
	thresholdMs := avgPerRune * float64(domain.LongTextSmoothThreshold)
	if rawMs <= thresholdMs {
		return rawMs
	}
	excessMs := rawMs - thresholdMs
	scale := math.Max(4500.0, profile.BaseIKIMs*35.0)
	return thresholdMs + scale*math.Log1p(excessMs/scale)
}

func (e *HSEMEngine) planAudioPTT(input domain.SimulationInput, plan *domain.FingerprintPlan, offset *time.Duration) {
	*offset += e.math.UniformDuration(domain.MinPreAction, domain.MaxPreAction)

	plan.Steps = append(plan.Steps, domain.ActionStep{
		Type:      domain.StepStateChange,
		Timestamp: *offset,
		Metadata:  "recording",
	})

	mediaDur := input.MediaDuration
	if mediaDur <= 0 {
		if input.FileSizeKB > 0 {
			estSec := math.Max(2.0, float64(input.FileSizeKB)/16.0)
			mediaDur = time.Duration(estSec * float64(time.Second))
		} else {
			mediaDur = 5 * time.Second
		}
	}

	humanMargin := e.math.UniformDuration(domain.AudioMarginMin, domain.AudioMarginMax)
	recordingTime := mediaDur + humanMargin
	*offset += recordingTime

	plan.Steps = append(plan.Steps, domain.ActionStep{
		Type:      domain.StepStateChange,
		Timestamp: *offset,
		Metadata:  "paused",
	})
	*offset += e.math.UniformDuration(150*time.Millisecond, 350*time.Millisecond)

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
	*offset += e.calculateUploadDuration(input.FileSizeKB, input.BandwidthKBps)

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

func (e *HSEMEngine) planDocument(input domain.SimulationInput, plan *domain.FingerprintPlan, offset *time.Duration) {
	plan.Steps = append(plan.Steps, domain.ActionStep{
		Type:      domain.StepStateChange,
		Timestamp: *offset,
		Metadata:  "composing",
	})

	searchTime := e.math.GaussianDuration(3000, 0.2)
	*offset += searchTime
	*offset += e.calculateUploadDuration(input.FileSizeKB, input.BandwidthKBps)

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

func (e *HSEMEngine) calculateUploadDuration(fileSizeKB int, sessionBandwidthKBps float64) time.Duration {
	rtt := e.math.UniformDuration(domain.UploadRTTMin, domain.UploadRTTMax)
	if fileSizeKB <= 0 {
		return rtt + e.math.UniformDuration(200*time.Millisecond, 600*time.Millisecond)
	}
	bw := sessionBandwidthKBps
	if bw <= 0 {
		bw = domain.DefaultBandwidthKBps
	}
	jitteredBW := bw * e.math.UniformFloat(0.80, 1.20)
	transferSec := float64(fileSizeKB) / jitteredBW
	transferDur := time.Duration(transferSec * float64(time.Second))
	return rtt + transferDur
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
	adjustedTime := input.CurrentTime.Add(input.CircadianOffset)
	hour := adjustedTime.Hour()

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

	if len([]rune(input.OutputText)) < 10 {
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

func clampFloat(v, minV, maxV float64) float64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}
