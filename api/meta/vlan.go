package meta

import (
	"sort"

	"github.com/pkg/errors"
)

type VLANRange struct {
	From uint16 `json:"from,omitempty"`
	To   uint16 `json:"to,omitempty"`
}

func SortVLANRanges(ranges []VLANRange) {
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].From == ranges[j].From {
			return ranges[i].To < ranges[j].To
		}
		return ranges[i].From < ranges[j].From
	})
}

func NormalizedVLANRanges(ranges []VLANRange) ([]VLANRange, error) {
	for idx := range ranges {
		if ranges[idx].To == 0 {
			ranges[idx].To = ranges[idx].From
		}
		if ranges[idx].From > ranges[idx].To {
			return nil, errors.Errorf("invalid range %d: from > to", idx)
		}
		if ranges[idx].From < 1 || ranges[idx].From > 4094 {
			return nil, errors.Errorf("invalid range %d: from < 1 || from > 4094", idx)
		}
		if ranges[idx].To < 1 || ranges[idx].To > 4094 {
			return nil, errors.Errorf("invalid range %d: to < 1 || to > 4094", idx)
		}
	}

	if len(ranges) < 2 {
		return ranges, nil
	}

	SortVLANRanges(ranges)

	res := []VLANRange{ranges[0]}
	for idx := 1; idx < len(ranges); idx++ {
		if res[len(res)-1].To >= ranges[idx].From {
			res[len(res)-1].To = ranges[idx].To
		} else {
			res = append(res, ranges[idx])
		}
	}

	return res, nil
}

func CheckVLANRangesOverlap(ranges []VLANRange) error {
	if len(ranges) < 2 {
		return nil
	}

	SortVLANRanges(ranges)

	for idx := 1; idx < len(ranges); idx++ {
		if ranges[idx-1].To >= ranges[idx].From {
			return errors.Errorf("VLAN ranges overlap: %d-%d and %d-%d", ranges[idx-1].From, ranges[idx-1].To, ranges[idx].From, ranges[idx].To)
		}
	}

	return nil
}
