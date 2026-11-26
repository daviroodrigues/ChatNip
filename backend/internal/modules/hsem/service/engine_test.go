package service

import (
	"chatnip/internal/modules/hsem/domain"
	"strings"
	"testing"
	"time"
)

func TestSmoothLongTextCurve_NoCliffAt200Chars(t *testing.T) {
	for _, persona := range []domain.PersonaType{
		domain.PersonaRushed,
		domain.PersonaStandard,
		domain.PersonaMeticulous,
		domain.PersonaDistracted,
	} {
		profile := domain.GetBiometricProfile(persona)
		raw199 := 199.0 * profile.BaseIKIMs
		raw200 := 200.0 * profile.BaseIKIMs
		raw500 := 500.0 * profile.BaseIKIMs

		s199 := smoothLongTextMs(raw199, 199, profile)
		s200 := smoothLongTextMs(raw200, 200, profile)
		s500 := smoothLongTextMs(raw500, 500, profile)

		if s200 <= s199 {
			t.Fatalf("persona %s: expected smooth monotonic increase at 199->200, got %.2f -> %.2f", persona, s199, s200)
		}
		if s500 <= s200 {
			t.Fatalf("persona %s: expected monotonic increase at 200->500, got %.2f -> %.2f", persona, s200, s500)
		}
		if s500 >= raw500 {
			t.Fatalf("persona %s: expected logarithmic compression for 500 chars, got %.2f >= %.2f", persona, s500, raw500)
		}
	}
}

func TestEmpiricalTransitions_ComplexTextCostsMoreThanLetters(t *testing.T) {
	engine := NewHSEMEngine(NewMathUtils())
	profile := domain.GetBiometricProfile(domain.PersonaStandard)

	plain := []rune(strings.Repeat("a", 80))
	complex := []rune(strings.Repeat("A1! 😊\n", 13) + "ab")

	_, plainMs := engine.computeEmpiricalSegments(plain, profile)
	_, complexMs := engine.computeEmpiricalSegments(complex, profile)

	if complexMs <= plainMs*1.5 {
		t.Fatalf("expected complex text (%.1fms) to cost significantly more than pure letters (%.1fms)", complexMs, plainMs)
	}
}

func TestEmpiricalPersonas_RushedFasterThanStandardAndDistracted(t *testing.T) {
	engine := NewHSEMEngine(NewMathUtils())
	sample := []rune("Ola, tudo bem? Gostaria de confirmar o pedido 49201 para amanha as 14:30!")

	_, rushedMs := engine.computeEmpiricalSegments(sample, domain.GetBiometricProfile(domain.PersonaRushed))
	_, stdMs := engine.computeEmpiricalSegments(sample, domain.GetBiometricProfile(domain.PersonaStandard))
	_, distMs := engine.computeEmpiricalSegments(sample, domain.GetBiometricProfile(domain.PersonaDistracted))

	if !(rushedMs < stdMs && stdMs < distMs) {
		t.Fatalf("expected empirical ordering rushed (%.1f) < standard (%.1f) < distracted (%.1f)", rushedMs, stdMs, distMs)
	}
}

func TestPlanAudioPTT_UsesRealDuration(t *testing.T) {
	engine := NewHSEMEngine(NewMathUtils())
	plan := engine.GenerateFingerprint(domain.SimulationInput{
		Persona:       domain.PersonaStandard,
		MessageType:   domain.MsgTypeAudioPTT,
		MediaDuration: 40 * time.Second,
		CurrentTime:   time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC),
	})

	var recTS, pauseTS time.Duration
	for _, s := range plan.Steps {
		if s.Metadata == "recording" {
			recTS = s.Timestamp
		}
		if s.Metadata == "paused" && recTS > 0 && pauseTS == 0 {
			pauseTS = s.Timestamp
		}
	}

	recDur := pauseTS - recTS
	if recDur < 40*time.Second+domain.AudioMarginMin || recDur > 40*time.Second+domain.AudioMarginMax {
		t.Fatalf("expected recording duration in [40.4s, 41.2s], got %v", recDur)
	}
}

func TestCalculateUploadDuration_ScalesWithBroadbandBandwidth(t *testing.T) {
	engine := NewHSEMEngine(NewMathUtils())
	slowDur := engine.calculateUploadDuration(51200, 2048)
	fastDur := engine.calculateUploadDuration(51200, 30720)

	if fastDur < domain.UploadRTTMin {
		t.Fatalf("expected upload duration >= RTT min (%v), got %v", domain.UploadRTTMin, fastDur)
	}
	if slowDur <= fastDur*5 {
		t.Fatalf("expected 2 MB/s upload (%v) to be much slower than 30 MB/s upload (%v)", slowDur, fastDur)
	}
}

func TestGenerateControlPlan_NonDeterministicHumanRanges(t *testing.T) {
	engine := NewHSEMEngine(NewMathUtils())
	baseInput := domain.SimulationInput{
		Persona:     domain.PersonaStandard,
		CurrentTime: time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC),
	}

	reactPlan := engine.GenerateControlPlan("reaction", baseInput)
	if len(reactPlan.Steps) != 1 || reactPlan.Steps[0].Timestamp < 900*time.Millisecond {
		t.Fatalf("expected reaction delay >= 900ms, got %+v", reactPlan)
	}

	revokePlan := engine.GenerateControlPlan("revoke", baseInput)
	if revokePlan.Steps[0].Timestamp < 1500*time.Millisecond || revokePlan.Steps[0].Timestamp > 4000*time.Millisecond {
		t.Fatalf("expected revoke delay in [1.5s, 4s], got %v", revokePlan.Steps[0].Timestamp)
	}

	readPlan := engine.GenerateControlPlan("mark_read", baseInput)
	if readPlan.Steps[0].Timestamp < 600*time.Millisecond || readPlan.Steps[0].Timestamp > 2200*time.Millisecond {
		t.Fatalf("expected mark_read delay in [600ms, 2.2s], got %v", readPlan.Steps[0].Timestamp)
	}
}

func TestCircadianOffsetAndSleepWindow(t *testing.T) {
	engine := NewHSEMEngine(NewMathUtils())
	t0550 := time.Date(2026, 11, 25, 5, 50, 0, 0, time.UTC)
	mNoShift := engine.calculateEnvironmentMultipliers(domain.SimulationInput{
		OutputText:      "mensagem de teste normal",
		CurrentTime:     t0550,
		CircadianOffset: 0,
	})
	mWithShift := engine.calculateEnvironmentMultipliers(domain.SimulationInput{
		OutputText:      "mensagem de teste normal",
		CurrentTime:     t0550,
		CircadianOffset: 20 * time.Minute,
	})
	if mNoShift != 1.5 || mWithShift != 1.1 {
		t.Fatalf("expected circadian shift to move 05:50 from 1.5x to 1.1x, got %.2f -> %.2f", mNoShift, mWithShift)
	}

	sleepPlan := engine.GenerateFingerprint(domain.SimulationInput{
		Persona:       domain.PersonaStandard,
		OutputText:    "oi",
		CurrentTime:   time.Date(2026, 11, 25, 3, 0, 0, 0, time.UTC),
		InSleepWindow: true,
	})
	if len(sleepPlan.Steps) == 0 || sleepPlan.Steps[0].Timestamp < domain.SleepInitialLatencyMin {
		t.Fatalf("expected sleep window initial latency >= %v, got %+v", domain.SleepInitialLatencyMin, sleepPlan.Steps)
	}
}

func TestSkipMarkAsReadAndCopyPasteProbability(t *testing.T) {
	engine := NewHSEMEngine(NewMathUtils())
	plan := engine.GenerateFingerprint(domain.SimulationInput{
		Persona:        domain.PersonaStandard,
		OutputText:     "segunda mensagem do lote",
		CurrentTime:    time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC),
		SkipMarkAsRead: true,
	})
	for _, s := range plan.Steps {
		if s.Metadata == "mark_as_read" {
			t.Fatalf("expected mark_as_read to be skipped when SkipMarkAsRead is true")
		}
	}

	longText := strings.Repeat("Este e um texto longo de demonstracao para testar copy paste. ", 7)
	copyPasteSeen := false
	for i := 0; i < 40; i++ {
		p := engine.GenerateFingerprint(domain.SimulationInput{
			Persona:     domain.PersonaStandard,
			OutputText:  longText,
			CurrentTime: time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC),
		})
		composingCount := 0
		var compStart, pauseAfter time.Duration
		for _, s := range p.Steps {
			if s.Metadata == "composing" {
				composingCount++
				compStart = s.Timestamp
			}
			if s.Metadata == "paused" && composingCount == 1 {
				pauseAfter = s.Timestamp
			}
		}
		if composingCount == 1 && (pauseAfter-compStart) <= domain.CopyPasteDurationMax+100*time.Millisecond {
			copyPasteSeen = true
			break
		}
	}
	if !copyPasteSeen {
		t.Fatalf("expected at least one copy-paste single-composing plan across 40 trials")
	}
}
