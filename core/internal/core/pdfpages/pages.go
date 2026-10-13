// Package pdfpages reads page count and MediaBox sizes from a PDF.
// It understands literal /MediaBox arrays. Indirect MediaBox references
// are rejected so a caller never stores a guessed page size.
package pdfpages

import (
	"bytes"
	"fmt"
	"strconv"
	"unicode"
)

// Size is one page's width and height in PDF points.
type Size struct {
	Width  float64
	Height float64
}

// Parse returns one size per page.
func Parse(pdf []byte) ([]Size, error) {
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		return nil, fmt.Errorf("not a PDF")
	}
	boxes := mediaBoxes(pdf)
	pages := countPages(pdf)
	if pages == 0 {
		pages = len(boxes)
	}
	if pages == 0 || len(boxes) == 0 {
		return nil, fmt.Errorf("PDF has no readable page size")
	}
	if len(boxes) == 1 && pages > 1 {
		out := make([]Size, pages)
		for i := range out {
			out[i] = boxes[0]
		}
		return out, nil
	}
	if len(boxes) < pages {
		return nil, fmt.Errorf("PDF page sizes are not all literal MediaBox values")
	}
	return boxes[:pages], nil
}

func countPages(pdf []byte) int {
	n := 0
	for i := 0; i+10 < len(pdf); i++ {
		if !bytes.HasPrefix(pdf[i:], []byte("/Type")) {
			continue
		}
		j := i + len("/Type")
		for j < len(pdf) && unicode.IsSpace(rune(pdf[j])) {
			j++
		}
		if j+5 <= len(pdf) && bytes.Equal(pdf[j:j+5], []byte("/Page")) {
			if j+5 < len(pdf) && pdf[j+5] == 's' {
				continue
			}
			n++
		}
	}
	return n
}

func mediaBoxes(pdf []byte) []Size {
	var out []Size
	needle := []byte("/MediaBox")
	for i := 0; i+len(needle) < len(pdf); i++ {
		if !bytes.HasPrefix(pdf[i:], needle) {
			continue
		}
		j := i + len(needle)
		for j < len(pdf) && unicode.IsSpace(rune(pdf[j])) {
			j++
		}
		if j >= len(pdf) || pdf[j] != '[' {
			continue
		}
		j++
		nums := make([]float64, 0, 4)
		for len(nums) < 4 && j < len(pdf) {
			for j < len(pdf) && (unicode.IsSpace(rune(pdf[j])) || pdf[j] == ',') {
				j++
			}
			end := j
			for end < len(pdf) && !unicode.IsSpace(rune(pdf[end])) && pdf[end] != ']' && pdf[end] != ',' {
				end++
			}
			if end == j {
				break
			}
			v, err := strconv.ParseFloat(string(pdf[j:end]), 64)
			if err != nil {
				break
			}
			nums = append(nums, v)
			j = end
		}
		if len(nums) != 4 {
			continue
		}
		w := nums[2] - nums[0]
		h := nums[3] - nums[1]
		if w <= 0 || h <= 0 {
			continue
		}
		out = append(out, Size{Width: w, Height: h})
	}
	return out
}
