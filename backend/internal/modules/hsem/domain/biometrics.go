package domain

import "unicode"

type CharCategory uint8

const (
	CatLetter CharCategory = iota
	CatUpper
	CatSpace
	CatNumber
	CatPunct
	CatEmoji
	CatNewline
)

type TransitionKey struct {
	From CharCategory
	To   CharCategory
}

type BiometricProfile struct {
	TTFKMedianMs         float64
	TTFKSigma            float64
	BaseIKIMs            float64
	IKISigma             float64
	BackspaceRate        float64
	CorrectionDurationMs float64
	ErrorPerceptionMs    float64
	AvgCharsPerBurst     float64
	EmojiCostMs          float64
	NewlineCostMs        float64
	TransitionsMs        map[TransitionKey]float64
}

var PersonaBiometrics = map[PersonaType]BiometricProfile{
	PersonaRushed: {
		TTFKMedianMs:         1053,
		TTFKSigma:            0.24,
		BaseIKIMs:            130,
		IKISigma:             0.32,
		BackspaceRate:        0.0769,
		CorrectionDurationMs: 337,
		ErrorPerceptionMs:    624,
		AvgCharsPerBurst:     5.3,
		EmojiCostMs:          750,
		NewlineCostMs:        700,
		TransitionsMs: map[TransitionKey]float64{
			{CatLetter, CatLetter}: 130,
			{CatLetter, CatSpace}:  158,
			{CatSpace, CatLetter}:  205,
			{CatSpace, CatUpper}:   291,
			{CatUpper, CatLetter}:  164,
			{CatLetter, CatPunct}:  247,
			{CatPunct, CatSpace}:   207,
			{CatPunct, CatLetter}:  250,
			{CatSpace, CatPunct}:   329,
			{CatSpace, CatNumber}:  643,
			{CatNumber, CatNumber}: 212,
			{CatNumber, CatPunct}:  497,
			{CatPunct, CatNumber}:  667,
			{CatNumber, CatSpace}:  277,
			{CatLetter, CatUpper}:  251,
			{CatUpper, CatUpper}:   85,
			{CatUpper, CatSpace}:   170,
			{CatUpper, CatPunct}:   206,
			{CatNumber, CatLetter}: 618,
			{CatLetter, CatNumber}: 313,
			{CatPunct, CatPunct}:   373,
		},
	},
	PersonaStandard: {
		TTFKMedianMs:         1041,
		TTFKSigma:            0.45,
		BaseIKIMs:            203,
		IKISigma:             0.38,
		BackspaceRate:        0.0599,
		CorrectionDurationMs: 518,
		ErrorPerceptionMs:    582,
		AvgCharsPerBurst:     3.7,
		EmojiCostMs:          900,
		NewlineCostMs:        850,
		TransitionsMs: map[TransitionKey]float64{
			{CatLetter, CatLetter}: 203,
			{CatLetter, CatSpace}:  209,
			{CatSpace, CatLetter}:  305,
			{CatSpace, CatUpper}:   353,
			{CatUpper, CatLetter}:  361,
			{CatLetter, CatPunct}:  741,
			{CatPunct, CatSpace}:   266,
			{CatPunct, CatLetter}:  316,
			{CatSpace, CatPunct}:   933,
			{CatSpace, CatNumber}:  646,
			{CatNumber, CatNumber}: 200,
			{CatNumber, CatPunct}:  526,
			{CatPunct, CatNumber}:  349,
			{CatNumber, CatSpace}:  477,
			{CatLetter, CatUpper}:  697,
			{CatUpper, CatUpper}:   181,
			{CatUpper, CatSpace}:   431,
			{CatUpper, CatPunct}:   375,
			{CatNumber, CatLetter}: 1039,
			{CatLetter, CatNumber}: 240,
			{CatPunct, CatPunct}:   593,
		},
	},
	PersonaMeticulous: {
		TTFKMedianMs:         1200,
		TTFKSigma:            0.38,
		BaseIKIMs:            208,
		IKISigma:             0.35,
		BackspaceRate:        0.0451,
		CorrectionDurationMs: 480,
		ErrorPerceptionMs:    665,
		AvgCharsPerBurst:     3.2,
		EmojiCostMs:          950,
		NewlineCostMs:        900,
		TransitionsMs: map[TransitionKey]float64{
			{CatLetter, CatLetter}: 208,
			{CatLetter, CatSpace}:  210,
			{CatSpace, CatLetter}:  367,
			{CatSpace, CatUpper}:   357,
			{CatUpper, CatLetter}:  427,
			{CatLetter, CatPunct}:  812,
			{CatPunct, CatSpace}:   312,
			{CatPunct, CatLetter}:  410,
			{CatSpace, CatPunct}:   1026,
			{CatSpace, CatNumber}:  748,
			{CatNumber, CatNumber}: 202,
			{CatNumber, CatPunct}:  392,
			{CatPunct, CatNumber}:  329,
			{CatNumber, CatSpace}:  496,
			{CatLetter, CatUpper}:  434,
			{CatUpper, CatUpper}:   311,
			{CatUpper, CatSpace}:   502,
			{CatUpper, CatPunct}:   483,
			{CatNumber, CatLetter}: 552,
			{CatLetter, CatNumber}: 280,
			{CatPunct, CatPunct}:   536,
		},
	},
	PersonaDistracted: {
		TTFKMedianMs:         2150,
		TTFKSigma:            0.55,
		BaseIKIMs:            265,
		IKISigma:             0.48,
		BackspaceRate:        0.1100,
		CorrectionDurationMs: 880,
		ErrorPerceptionMs:    1750,
		AvgCharsPerBurst:     2.2,
		EmojiCostMs:          1150,
		NewlineCostMs:        1050,
		TransitionsMs: map[TransitionKey]float64{
			{CatLetter, CatLetter}: 265,
			{CatLetter, CatSpace}:  295,
			{CatSpace, CatLetter}:  420,
			{CatSpace, CatUpper}:   620,
			{CatUpper, CatLetter}:  350,
			{CatLetter, CatPunct}:  890,
			{CatPunct, CatSpace}:   360,
			{CatPunct, CatLetter}:  620,
			{CatSpace, CatPunct}:   780,
			{CatSpace, CatNumber}:  860,
			{CatNumber, CatNumber}: 380,
			{CatNumber, CatPunct}:  780,
			{CatPunct, CatNumber}:  720,
			{CatNumber, CatSpace}:  450,
			{CatLetter, CatUpper}:  710,
			{CatUpper, CatUpper}:   410,
			{CatUpper, CatSpace}:   460,
			{CatUpper, CatPunct}:   760,
			{CatNumber, CatLetter}: 650,
			{CatLetter, CatNumber}: 580,
			{CatPunct, CatPunct}:   540,
		},
	},
}

func GetBiometricProfile(p PersonaType) BiometricProfile {
	if prof, ok := PersonaBiometrics[p]; ok {
		return prof
	}
	return PersonaBiometrics[PersonaStandard]
}

func (p BiometricProfile) TransitionCostMs(from, to CharCategory) float64 {
	if to == CatEmoji {
		return p.EmojiCostMs
	}
	if to == CatNewline {
		return p.NewlineCostMs
	}
	if from == CatEmoji || from == CatNewline {
		from = CatSpace
	}
	if val, ok := p.TransitionsMs[TransitionKey{From: from, To: to}]; ok {
		return val
	}
	return p.BaseIKIMs
}

func ClassifyRune(r rune) CharCategory {
	switch {
	case r == '\n' || r == '\r':
		return CatNewline
	case unicode.IsSpace(r):
		return CatSpace
	case unicode.IsDigit(r):
		return CatNumber
	case unicode.IsUpper(r):
		return CatUpper
	case unicode.IsLower(r) || unicode.IsLetter(r):
		return CatLetter
	case isEmojiRune(r):
		return CatEmoji
	default:
		return CatPunct
	}
}

func isEmojiRune(r rune) bool {
	return (r >= 0x1F300 && r <= 0x1FAFF) ||
		(r >= 0x2600 && r <= 0x27BF) ||
		(r >= 0xFE00 && r <= 0xFE0F)
}
