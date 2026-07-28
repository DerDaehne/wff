package analyze

import "fmt"

// "How much did I burn?" is one of the first questions a recreational rider
// asks, and with a pulse trace it is answerable (#625).
//
// The estimate follows Keytel et al., Journal of Sports Sciences 2005 — a
// regression of energy expenditure on heart rate, body weight, age and sex,
// fitted on treadmill and cycle-ergometer work. It is the approximation bike
// computers and watches use too.
//
// Two properties matter for how it is used here:
//   - It is linear in heart rate, so integrating over the trace gives exactly
//     the same answer as using the ride's average. Unlike the training load
//     (#622) there is nothing to gain from the sample-by-sample route.
//   - It was fitted on people who were exercising. Below roughly 90 bpm it
//     leaves the range it was built for and starts returning negative numbers,
//     so there is a floor rather than a clamp: a figure that came out of an
//     extrapolation is not worth showing.

// Sex values, as stored. The set has two entries because the source publishes
// two coefficient sets, not because the app has an opinion — anything else,
// including nothing, means no calorie estimate.
const (
	SexMale   = "male"
	SexFemale = "female"
)

const (
	// caloriesMinHeartRate is where the regression leaves the range it was
	// fitted in. Coasting downhill at 80 bpm is not burning negative energy.
	caloriesMinHeartRate = 90
	// kjPerKcal converts the formula's kilojoules into the unit on the label of
	// every food packet.
	kjPerKcal = 4.184
)

// Calories estimates the energy a ride cost, in kcal. The bool is false
// whenever an input is missing or out of the range the formula covers — the
// caller shows nothing then, rather than a number built on a guess.
func Calories(avgHeartRateBpm float64, seconds int, weightKg float64, age int, sex string) (int, bool) {
	if avgHeartRateBpm < caloriesMinHeartRate || seconds <= 0 || weightKg <= 0 || age <= 0 {
		return 0, false
	}

	var kjPerMinute float64
	switch sex {
	case SexMale:
		kjPerMinute = -55.0969 + 0.6309*avgHeartRateBpm + 0.1988*weightKg + 0.2017*float64(age)
	case SexFemale:
		kjPerMinute = -20.4022 + 0.4472*avgHeartRateBpm - 0.1263*weightKg + 0.0740*float64(age)
	default:
		return 0, false
	}
	if kjPerMinute <= 0 {
		return 0, false
	}

	kcal := kjPerMinute / kjPerKcal * (float64(seconds) / 60)
	return int(kcal + 0.5), true
}

// calories is the ride's energy estimate, over moving time rather than elapsed:
// standing at a traffic light is not what cost the calories.
func (f RideFacts) calories() (kcal, age int, ok bool) {
	age, ok = f.ageAtRide()
	if !ok || f.AvgHeartRate == nil || f.WeightKg == nil || f.Sex == nil {
		return 0, 0, false
	}
	kcal, ok = Calories(*f.AvgHeartRate, f.MovingSeconds, *f.WeightKg, age, *f.Sex)
	return kcal, age, ok
}

// caloriesStatement says the number and, in the same breath, how firm it is.
// A pulse-derived calorie figure is an estimate wearing the clothes of a
// measurement, and a rider who plans their eating around it should know that.
func caloriesStatement(kcal int, age int) Statement {
	return Statement{
		Text: fmt.Sprintf("Diese Fahrt hat dich etwa %d Kilokalorien gekostet. Das ist eine "+
			"Schätzung aus deinem Puls, nicht eine Messung — je nach Tagesform liegt der "+
			"echte Wert gut ein Fünftel darüber oder darunter.", kcal),
		Metric: fmt.Sprintf("aus Puls, Gewicht und Alter (%d) · Formel nach Keytel", age),
		Kind:   "calories",
	}
}
