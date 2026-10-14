package domain

import (
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

const (
	// MinEstimateMinutes is the smallest user-created estimate Momentum accepts.
	MinEstimateMinutes = 1
	// MaxEstimateMinutes is the largest user-created estimate Momentum accepts.
	MaxEstimateMinutes = 24 * 60
)

var userEstimatePattern = regexp.MustCompile(`(?i)^(?:(\d+(?:\.\d+)?)h)?(?:(\d+)m)?$`)

// Estimate is focused task effort measured in whole minutes. It is not a
// calendar interval and never represents a start or end time.
type Estimate struct {
	Minutes int
}

// Validate reports whether an estimate can be represented as positive whole
// minutes. Exported Taskwarrior values may exceed MaxEstimateMinutes, so the
// user-input bound is checked separately by ParseEstimate.
func (e Estimate) Validate() error {
	if e.Minutes < MinEstimateMinutes {
		return fmt.Errorf("estimate must be at least %d minute", MinEstimateMinutes)
	}
	return nil
}

// ValidateForCreation applies Momentum's user-facing one-day limit.
func (e Estimate) ValidateForCreation() error {
	if err := e.Validate(); err != nil {
		return err
	}
	if e.Minutes > MaxEstimateMinutes {
		return fmt.Errorf("estimate cannot exceed 24 hours (%d minutes)", MaxEstimateMinutes)
	}
	return nil
}

// ParseEstimate parses Momentum's compact user syntax. Hours may be decimal
// when they resolve exactly to whole minutes; minutes are always integral.
// It enforces the 24-hour bound for new or explicitly edited values.
func ParseEstimate(value string) (Estimate, error) {
	return parseHumanEstimate(value, true)
}

// ParseEstimateValue parses the same compact syntax without the creation
// bound. It is used only to preserve an externally created value that is over
// 24 hours while a task is opened or compared in the editor.
func ParseEstimateValue(value string) (Estimate, error) {
	return parseHumanEstimate(value, false)
}

func parseHumanEstimate(value string, enforceLimit bool) (Estimate, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return Estimate{}, fmt.Errorf("estimate is required; use values such as 15m, 1h, or 1h30m")
	}
	total, err := parseUserEstimate(text)
	if err != nil {
		return Estimate{}, fmt.Errorf("invalid estimate %q: %w", text, err)
	}
	estimate, err := estimateFromBigInt(total)
	if err != nil {
		return Estimate{}, fmt.Errorf("invalid estimate %q: %w", text, err)
	}
	if enforceLimit {
		if err := estimate.ValidateForCreation(); err != nil {
			return Estimate{}, fmt.Errorf("invalid estimate %q: %w", text, err)
		}
	} else if err := estimate.Validate(); err != nil {
		return Estimate{}, fmt.Errorf("invalid estimate %q: %w", text, err)
	}
	return estimate, nil
}

// ParseTaskwarriorEstimate parses the precise ISO-8601 duration forms emitted
// by a duration UDA. Calendar months and years are deliberately rejected;
// days, hours, minutes, and seconds are accepted only when they resolve to a
// positive whole number of minutes.
func ParseTaskwarriorEstimate(value string) (Estimate, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return Estimate{}, fmt.Errorf("duration is empty")
	}
	upper := strings.ToUpper(text)
	if strings.HasPrefix(upper, "-") {
		return Estimate{}, fmt.Errorf("negative durations are not supported for estimates")
	}
	if !strings.HasPrefix(upper, "P") {
		return Estimate{}, fmt.Errorf("expected an ISO-8601 duration beginning with P")
	}
	body := upper[1:]
	if body == "" {
		return Estimate{}, fmt.Errorf("duration has no components")
	}
	datePart, timePart := body, ""
	if index := strings.IndexByte(body, 'T'); index >= 0 {
		datePart = body[:index]
		timePart = body[index+1:]
		if timePart == "" {
			return Estimate{}, fmt.Errorf("duration has no time components")
		}
	}
	if datePart == "" && timePart == "" {
		return Estimate{}, fmt.Errorf("duration has no components")
	}

	seconds := new(big.Int)
	if datePart != "" {
		if err := parseISOSection(datePart, map[byte]isoComponent{
			'D': {Rank: 1, Seconds: 24 * 60 * 60},
		}, seconds, true); err != nil {
			return Estimate{}, err
		}
	}
	if timePart != "" {
		if err := parseISOSection(timePart, map[byte]isoComponent{
			'H': {Rank: 3, Seconds: 60 * 60},
			'M': {Rank: 2, Seconds: 60},
			'S': {Rank: 1, Seconds: 1},
		}, seconds, false); err != nil {
			return Estimate{}, err
		}
	}
	if seconds.Sign() <= 0 {
		return Estimate{}, fmt.Errorf("duration must be positive")
	}
	minutes, remainder := new(big.Int), new(big.Int)
	minutes.QuoRem(seconds, big.NewInt(60), remainder)
	if remainder.Sign() != 0 {
		return Estimate{}, fmt.Errorf("duration must resolve to whole minutes; seconds are not supported unless divisible by 60")
	}
	return estimateFromBigInt(minutes)
}

type isoComponent struct {
	Rank    int
	Seconds int64
}

func parseISOSection(section string, components map[byte]isoComponent, total *big.Int, calendar bool) error {
	lastRank := int(^uint(0) >> 1)
	index := 0
	seen := make(map[byte]struct{}, len(components))
	for index < len(section) {
		start := index
		for index < len(section) && section[index] >= '0' && section[index] <= '9' {
			index++
		}
		if start == index {
			return fmt.Errorf("invalid ISO duration component near %q", section[start:])
		}
		if index >= len(section) {
			return fmt.Errorf("invalid ISO duration component near %q", section[start:])
		}
		unit := section[index]
		index++
		component, ok := components[unit]
		if !ok {
			if calendar && (unit == 'M' || unit == 'Y') {
				return fmt.Errorf("calendar months and years are not supported for focused estimates")
			}
			return fmt.Errorf("unsupported ISO duration unit %q", string(unit))
		}
		if _, ok := seen[unit]; ok || component.Rank >= lastRank {
			return fmt.Errorf("ISO duration components must be ordered and unique")
		}
		seen[unit] = struct{}{}
		lastRank = component.Rank
		value, ok := new(big.Int).SetString(section[start:index-1], 10)
		if !ok {
			return fmt.Errorf("invalid ISO duration number %q", section[start:index-1])
		}
		multiplier := big.NewInt(component.Seconds)
		value.Mul(value, multiplier)
		total.Add(total, value)
	}
	return nil
}

func parseUserEstimate(value string) (*big.Int, error) {
	matches := userEstimatePattern.FindStringSubmatch(value)
	if matches == nil || (matches[1] == "" && matches[2] == "") {
		return nil, fmt.Errorf("use 15m, 90m, 1h, 1.5h, or 1h30m")
	}
	if matches[1] != "" && matches[2] != "" && strings.Contains(matches[1], ".") {
		return nil, fmt.Errorf("fractional hours cannot be combined with minutes")
	}

	total := new(big.Int)
	if matches[1] != "" {
		hours, ok := new(big.Rat).SetString(matches[1])
		if !ok {
			return nil, fmt.Errorf("invalid hour value")
		}
		minutes := new(big.Rat).Mul(hours, big.NewRat(60, 1))
		if minutes.Denom().Cmp(big.NewInt(1)) != 0 {
			return nil, fmt.Errorf("duration must resolve to whole minutes")
		}
		total.Set(minutes.Num())
	}
	if matches[2] != "" {
		minutes, ok := new(big.Int).SetString(matches[2], 10)
		if !ok {
			return nil, fmt.Errorf("invalid minute value")
		}
		total.Add(total, minutes)
	}
	return total, nil
}

func estimateFromBigInt(value *big.Int) (Estimate, error) {
	if value == nil || value.Sign() <= 0 {
		return Estimate{}, fmt.Errorf("duration must be positive")
	}
	if !value.IsInt64() {
		return Estimate{}, fmt.Errorf("duration is too large")
	}
	minutes := value.Int64()
	maxInt := int64(^uint(0) >> 1)
	if minutes > maxInt {
		return Estimate{}, fmt.Errorf("duration is too large")
	}
	return Estimate{Minutes: int(minutes)}, nil
}

// TaskwarriorValue returns the unambiguous minute spelling required by
// Taskwarrior 3.x. Invalid values return an empty string and should be
// rejected by Validate before mutation.
func (e Estimate) TaskwarriorValue() string {
	if e.Validate() != nil {
		return ""
	}
	return strconv.Itoa(e.Minutes) + "min"
}

// String formats an estimate for Momentum's compact human-facing surfaces.
func (e Estimate) String() string {
	if e.Validate() != nil {
		return ""
	}
	hours := e.Minutes / 60
	minutes := e.Minutes % 60
	if hours == 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

// InputValue formats an estimate in the compact syntax ParseEstimateValue
// accepts, so an editable field opens with a value that round-trips.
func (e Estimate) InputValue() string {
	return strings.ReplaceAll(e.String(), " ", "")
}

// EstimateSuggestions returns the deterministic quick-capture/editor presets.
func EstimateSuggestions() []string {
	return []string{"15m", "30m", "45m", "1h", "2h", "4h"}
}
