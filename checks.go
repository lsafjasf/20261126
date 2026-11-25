package clipmerge

import "fmt"

// Check names in canonical evaluation order.
const (
	CheckSourceInBounds      = "source_in_bounds"
	CheckSourceOutBounds     = "source_out_bounds"
	CheckSourceIntervalOrder = "source_interval_order"
	CheckSpeedPositive       = "speed_positive"
	CheckTimelineDurationMin = "timeline_duration_min"
)

const (
	checkStatusPass    = "pass"
	checkStatusFail    = "fail"
	checkStatusSkipped = "skipped"

	checkMessageSatisfied = "constraint satisfied"
	checkMessageSkipped   = "not evaluated because property values are unresolved or stale"
)

// checkMeta describes a boundary check: the properties it depends on and
// its human-readable rule. Dependencies are what make a failure a
// property-level dependency conflict.
type checkMeta struct {
	name       string
	properties []string
	rule       string
}

// checkOrder is the deterministic evaluation order of all boundary checks.
var checkOrder = []checkMeta{
	{CheckSourceInBounds, []string{PropertyInPoint}, "source in-point must lie inside the media"},
	{CheckSourceOutBounds, []string{PropertyOutPoint}, "source out-point must lie inside the media"},
	{CheckSourceIntervalOrder, []string{PropertyInPoint, PropertyOutPoint}, "source in-point must precede source out-point"},
	{CheckSpeedPositive, []string{PropertySpeed}, "speed must be a positive rational"},
	{CheckTimelineDurationMin, []string{PropertyInPoint, PropertyOutPoint, PropertySpeed}, "the combined source span and speed must yield a long enough timeline duration"},
}

// skippedChecks returns every check in the skipped state, used when property
// resolution failed and no combined value may be evaluated.
func skippedChecks() []Check {
	checks := make([]Check, 0, len(checkOrder))
	for _, meta := range checkOrder {
		checks = append(checks, Check{
			Name: meta.name, Properties: meta.properties, Rule: meta.rule,
			Status: checkStatusSkipped, Observed: "", Expected: "",
			Message: checkMessageSkipped,
		})
	}
	return checks
}

// runChecks evaluates every media-boundary check against the merged clip.
// The clip here is always fully resolved; gating happens in the engine.
func runChecks(src Source, c Clip) []Check {
	checks := make([]Check, 0, len(checkOrder))
	for _, meta := range checkOrder {
		checks = append(checks, runCheck(src, c, meta))
	}
	return checks
}

func runCheck(src Source, c Clip, meta checkMeta) Check {
	chk := Check{Name: meta.name, Properties: meta.properties, Rule: meta.rule}
	pass := true
	switch meta.name {
	case CheckSourceInBounds:
		chk.Observed = fmt.Sprintf("inPoint=%d", c.InPoint)
		chk.Expected = fmt.Sprintf("0 <= inPoint <= %d", src.Duration)
		pass = 0 <= c.InPoint && c.InPoint <= src.Duration
		if !pass {
			chk.Message = fmt.Sprintf("inPoint=%d violates 0 <= inPoint <= %d", c.InPoint, src.Duration)
		}
	case CheckSourceOutBounds:
		chk.Observed = fmt.Sprintf("outPoint=%d", c.OutPoint)
		chk.Expected = fmt.Sprintf("0 <= outPoint <= %d", src.Duration)
		pass = 0 <= c.OutPoint && c.OutPoint <= src.Duration
		if !pass {
			chk.Message = fmt.Sprintf("outPoint=%d violates 0 <= outPoint <= %d", c.OutPoint, src.Duration)
		}
	case CheckSourceIntervalOrder:
		chk.Observed = fmt.Sprintf("inPoint=%d, outPoint=%d", c.InPoint, c.OutPoint)
		chk.Expected = "inPoint < outPoint"
		pass = c.InPoint < c.OutPoint
		if !pass {
			chk.Message = fmt.Sprintf("inPoint=%d is not before outPoint=%d", c.InPoint, c.OutPoint)
		}
	case CheckSpeedPositive:
		spd := c.Speed.Normalized()
		chk.Observed = fmt.Sprintf("speed=%s", spd.String())
		chk.Expected = "speed > 0"
		pass = spd.Numerator > 0
		if !pass {
			chk.Message = fmt.Sprintf("speed=%s is not positive", spd.String())
		}
	case CheckTimelineDurationMin:
		dur := c.TimelineDuration()
		chk.Observed = fmt.Sprintf("timelineDuration=%s, minTimelineDuration=%d", dur.String(), src.MinTimelineDuration)
		chk.Expected = fmt.Sprintf("timelineDuration >= %d", src.MinTimelineDuration)
		pass = dur.Numerator >= src.MinTimelineDuration*dur.Denominator
		if !pass {
			chk.Message = fmt.Sprintf("combined inPoint=%d, outPoint=%d and speed=%s yield timeline duration %s, below minimum %d",
				c.InPoint, c.OutPoint, c.Speed.Normalized().String(), dur.String(), src.MinTimelineDuration)
		}
	}
	if pass {
		chk.Status = checkStatusPass
		chk.Message = checkMessageSatisfied
	} else {
		chk.Status = checkStatusFail
	}
	return chk
}
