package service

import (
	"math"
	"math/rand/v2"
	"time"
)

type MathUtils struct{}

func NewMathUtils() *MathUtils {
	return &MathUtils{}
}

func (m *MathUtils) GaussianDuration(meanMs float64, variancePct float64) time.Duration {
	stdDev := meanMs * variancePct
	minLimit := 50.0

	var val float64
	for i := 0; i < 3; i++ {
		val = rand.NormFloat64()*stdDev + meanMs
		if val >= 20 {
			break
		}
	}

	if val < minLimit {
		fuzz := rand.Float64() * 35.0
		val = minLimit + fuzz
	}

	return time.Duration(val) * time.Millisecond
}

func (m *MathUtils) LogNormalDuration(medianMs float64, sigma float64, minDur, maxDur time.Duration) time.Duration {
	if medianMs <= 0 {
		return minDur
	}
	if sigma <= 0 {
		sigma = 0.25
	}
	z := rand.NormFloat64()
	if z > 2.5 {
		z = 2.5
	} else if z < -2.5 {
		z = -2.5
	}
	valMs := medianMs * math.Exp(sigma*z)
	dur := time.Duration(valMs) * time.Millisecond
	if minDur > 0 && dur < minDur {
		return minDur
	}
	if maxDur > 0 && dur > maxDur {
		return maxDur
	}
	return dur
}

func (m *MathUtils) UniformDuration(min, max time.Duration) time.Duration {
	delta := int64(max - min)
	if delta <= 0 {
		return min
	}
	return min + time.Duration(rand.Int64N(delta))
}

func (m *MathUtils) UniformFloat(min, max float64) float64 {
	if max <= min {
		return min
	}
	return min + rand.Float64()*(max-min)
}

func (m *MathUtils) ExponentialDuration(min, max time.Duration) time.Duration {
	lambda := 0.5
	exp := rand.ExpFloat64() / lambda
	durationMs := float64(min.Milliseconds()) + (exp * 1000)

	maxMs := float64(max.Milliseconds())
	if durationMs > maxMs {
		excess := durationMs - maxMs
		durationMs = maxMs + (excess / 10.0)
	}

	return time.Duration(durationMs) * time.Millisecond
}

func (m *MathUtils) ShouldHappen(probability float64) bool {
	return rand.Float64() < probability
}
