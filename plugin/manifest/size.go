package manifest

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// PopupSize is one dimension of a popup pane: a count of outer terminal cells
// or a percentage of the terminal area. It mirrors herdr's PopupSize
// (src/popup_size.rs), which accepts an integer or a string like "80%".
type PopupSize struct {
	// Cells is the size in outer terminal cells, border included. It is only
	// meaningful when Percent is zero.
	Cells uint16
	// Percent is the size as a percentage of the terminal area, 1 to 100.
	// Zero means the size is a cell count.
	Percent uint8
}

// IsPercent reports whether the size is a percentage of the terminal area.
func (s PopupSize) IsPercent() bool { return s.Percent != 0 }

// String returns the manifest spelling of the size.
func (s PopupSize) String() string {
	if s.IsPercent() {
		return strconv.FormatUint(uint64(s.Percent), 10) + "%"
	}
	return strconv.FormatUint(uint64(s.Cells), 10)
}

// UnmarshalTOML decodes an integer cell count or a percentage string.
func (s *PopupSize) UnmarshalTOML(data any) error {
	switch value := data.(type) {
	case int64:
		if value < 0 || value > math.MaxUint16 {
			return fmt.Errorf("popup size %d is out of range; a cell count is 0 to %d", value, math.MaxUint16)
		}
		*s = PopupSize{Cells: uint16(value)}
		return nil
	case string:
		digits, ok := strings.CutSuffix(value, "%")
		if !ok {
			return fmt.Errorf("popup size %q must be a percentage like \"80%%\"; use an integer for cells", value)
		}
		percent, err := strconv.ParseUint(digits, 10, 8)
		if err != nil || percent < 1 || percent > 100 {
			return fmt.Errorf("popup size %q must be a percentage between 1%% and 100%%", value)
		}
		*s = PopupSize{Percent: uint8(percent)}
		return nil
	default:
		return fmt.Errorf("popup size must be an integer or a percentage like \"80%%\", got %T", data)
	}
}
