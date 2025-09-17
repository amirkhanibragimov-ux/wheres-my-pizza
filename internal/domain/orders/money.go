package orders

import "math"

// Money represents currency in minor units (cents) to avoid float issues. DB adapters convert to/from NUMERIC(10,2).
type Money int64

// NewMoneyFromFloat2 creates Money from float64 dollars, rounding to nearest cent.
func NewMoneyFromFloat2(f float64) Money {
	return Money(math.Round(f * 100.0))
}

func (m Money) ToFloat2() float64 { return float64(m) / 100.0 }
