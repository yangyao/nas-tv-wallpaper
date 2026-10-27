package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	_ "image/jpeg"
	"io"
	"os"
	"strings"
)

type PhotoMeta struct {
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Orientation int    `json:"orientation"`
	Make        string `json:"make,omitempty"`
	Model       string `json:"model,omitempty"`
	Taken       string `json:"taken,omitempty"`
	Exposure    bool   `json:"exposure"`
}

// Only a sampled file's bounded JPEG header is read; never a library-wide metadata scan.
func metadata(path string) (PhotoMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return PhotoMeta{}, err
	}
	defer f.Close()
	header, err := io.ReadAll(io.LimitReader(f, 1<<20))
	if err != nil {
		return PhotoMeta{}, err
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(header))
	if err != nil || format != "jpeg" {
		return PhotoMeta{}, errors.New("not a supported JPEG")
	}
	m := PhotoMeta{Width: cfg.Width, Height: cfg.Height, Orientation: 1}
	if len(header) < 4 || header[0] != 0xff || header[1] != 0xd8 {
		return m, errors.New("invalid JPEG")
	}
	for pos := 2; pos+4 <= len(header); {
		if header[pos] != 0xff {
			break
		}
		marker := header[pos+1]
		pos += 2
		if marker == 0xda || marker == 0xd9 {
			break
		}
		if marker == 0x00 || marker == 0xff {
			continue
		}
		n := int(binary.BigEndian.Uint16(header[pos : pos+2]))
		if n < 2 || pos+n > len(header) {
			break
		}
		section := header[pos+2 : pos+n]
		if marker == 0xe1 && bytes.HasPrefix(section, []byte("Exif\x00\x00")) {
			readTIFF(section[6:], &m)
			break
		}
		pos += n
	}
	return m, nil
}
func readTIFF(b []byte, m *PhotoMeta) {
	if len(b) < 8 {
		return
	}
	var order binary.ByteOrder
	if string(b[:2]) == "II" {
		order = binary.LittleEndian
	} else if string(b[:2]) == "MM" {
		order = binary.BigEndian
	} else {
		return
	}
	if order.Uint16(b[2:4]) != 42 {
		return
	}
	seen := map[uint32]bool{}
	var visit func(uint32, int)
	visit = func(offset uint32, depth int) {
		if depth > 2 || seen[offset] || uint64(offset)+2 > uint64(len(b)) {
			return
		}
		seen[offset] = true
		n := int(order.Uint16(b[offset : offset+2]))
		if n > 512 {
			return
		}
		for i := 0; i < n; i++ {
			p := int(offset) + 2 + i*12
			if p+12 > len(b) {
				return
			}
			tag, kind, count := order.Uint16(b[p:p+2]), order.Uint16(b[p+2:p+4]), order.Uint32(b[p+4:p+8])
			value := order.Uint32(b[p+8 : p+12])
			if tag == 0x8769 && kind == 4 {
				visit(value, depth+1)
				continue
			}
			if tag == 0x112 && kind == 3 && count == 1 {
				v := int(order.Uint16(b[p+8 : p+10]))
				if v >= 1 && v <= 8 {
					m.Orientation = v
				}
			}
			if tag == 0x829a || tag == 0x829d || tag == 0x8827 || tag == 0x920a {
				if count > 0 {
					m.Exposure = true
				}
			}
			if kind == 2 && count > 0 && count <= 256 {
				start := p + 8
				if count > 4 {
					start = int(value)
				}
				if start < 0 || uint64(start)+uint64(count) > uint64(len(b)) {
					continue
				}
				s := strings.TrimSpace(strings.TrimRight(string(b[start:start+int(count)]), "\x00"))
				switch tag {
				case 0x10f:
					m.Make = s
				case 0x110:
					m.Model = s
				case 0x9003:
					m.Taken = s
				}
			}
		}
	}
	visit(order.Uint32(b[4:8]), 0)
}
func eligible(m PhotoMeta) bool {
	// Conservative: camera metadata plus an exposure-related tag. Missing metadata stays out.
	if m.Width < 600 || m.Height < 600 || maxInt(m.Width, m.Height) < 960 || int64(m.Width)*int64(m.Height) > 40_000_000 {
		return false
	}
	return m.Model != "" && len(m.Taken) >= 10 && m.Exposure
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
