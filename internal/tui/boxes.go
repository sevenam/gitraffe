package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// truncateLines truncates each line of s to maxWidth visible characters,
// correctly handling ANSI escape sequences.
func truncateLines(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if ansi.StringWidth(line) > maxWidth {
			lines[i] = ansi.Truncate(line, maxWidth, "")
		}
	}
	return strings.Join(lines, "\n")
}

// trimToHeight ensures a rendered string is exactly targetHeight lines.
// If taller, excess lines are removed from the bottom (preserving the bottom border).
// If shorter, empty lines are appended.
func trimToHeight(rendered string, targetHeight int) string {
	lines := strings.Split(rendered, "\n")
	// Remove trailing empty string from split if present
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > targetHeight {
		// Keep first line (top border), middle content up to targetHeight-2, and last line (bottom border)
		top := lines[0]
		bottom := lines[len(lines)-1]
		middle := lines[1 : targetHeight-1]
		result := make([]string, 0, targetHeight)
		result = append(result, top)
		result = append(result, middle...)
		result = append(result, bottom)
		return strings.Join(result, "\n")
	}
	for len(lines) < targetHeight {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// addBoxLabel overlays a label like [0] onto the top-left corner of a rendered box border.
// It accounts for ANSI escape sequences so it only replaces visible border characters.
func addBoxLabel(rendered string, label string) string {
	lines := strings.SplitN(rendered, "\n", 2)
	if len(lines) == 0 {
		return rendered
	}
	topLine := lines[0]
	labelRunes := []rune(label)

	// Walk through the top line, skipping ANSI escape sequences,
	// and replace visible characters at positions 1..len(label) (after the corner char).
	var result strings.Builder
	visibleIdx := 0
	labelIdx := 0
	runes := []rune(topLine)
	for i := 0; i < len(runes); i++ {
		// Detect ANSI escape sequence: ESC [ params final_byte
		if runes[i] == '\033' && i+1 < len(runes) && runes[i+1] == '[' {
			// Copy ESC and [ first
			result.WriteRune(runes[i]) // \033
			i++
			result.WriteRune(runes[i]) // [
			i++
			// Now copy parameter/intermediate bytes until final byte (0x40-0x7E)
			for i < len(runes) {
				result.WriteRune(runes[i])
				if runes[i] >= 0x40 && runes[i] <= 0x7E {
					break
				}
				i++
			}
			continue
		}
		// This is a visible character
		if visibleIdx >= 1 && labelIdx < len(labelRunes) {
			result.WriteRune(labelRunes[labelIdx])
			labelIdx++
		} else {
			result.WriteRune(runes[i])
		}
		visibleIdx++
	}

	lines[0] = result.String()
	if len(lines) > 1 {
		return lines[0] + "\n" + lines[1]
	}
	return lines[0]
}

// overlayCentre draws box over the middle of screen, leaving the screen visible
// around it so the help reads as a layer over the app rather than a new page.
// lipgloss v1 has no compositing, so each line is spliced by display column:
// the screen's left part, the box's line, then the screen's right part. The
// box is clipped when the screen is smaller than it.
func overlayCentre(screen, box string, width, height int) string {
	lines := strings.Split(screen, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	boxLines := strings.Split(box, "\n")
	if len(boxLines) > height {
		boxLines = boxLines[:height]
	}
	boxWidth := 0
	for _, l := range boxLines {
		boxWidth = max(boxWidth, ansi.StringWidth(l))
	}
	boxWidth = min(boxWidth, width)

	top := (height - len(boxLines)) / 2
	left := (width - boxWidth) / 2
	for i, bl := range boxLines {
		row := top + i
		bg := lines[row]
		// Pad a short screen line so the right-hand cut lands in the same column.
		if w := ansi.StringWidth(bg); w < width {
			bg += strings.Repeat(" ", width-w)
		}
		bl = ansi.Truncate(bl, boxWidth, "")
		if w := ansi.StringWidth(bl); w < boxWidth {
			bl += strings.Repeat(" ", boxWidth-w)
		}
		// The resets stop a colour left open in the screen's left part bleeding
		// into the box, and the box's colour into the screen's right part.
		lines[row] = ansi.Truncate(bg, left, "") + "\x1b[0m" + bl + "\x1b[0m" + ansi.TruncateLeft(bg, left+boxWidth, "")
	}
	return strings.Join(lines, "\n")
}
