package relay

import (
	"bufio"
	"encoding/base64"
	"errors"
	"io"
	"sort"

	"github.com/QuantumNous/new-api/common"
)

// Normalized image JSON can contain arbitrarily large base64 strings. Index
// byte ranges in the private spool instead of decoding media into heap objects.
// Provider adapters remain responsible for protocol validation and usage.
type imageJSONValue struct {
	source      io.ReaderAt
	start, size int64
	literal     []byte
	base64      bool
}

func (v imageJSONValue) reader() io.Reader { return io.NewSectionReader(v.source, v.start, v.size) }
func (v imageJSONValue) write(w io.Writer) error {
	if v.base64 {
		if _, err := io.WriteString(w, "\""); err != nil {
			return err
		}
		encoder := base64.NewEncoder(base64.StdEncoding, w)
		_, err := io.Copy(encoder, v.reader())
		closeErr := encoder.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		_, err = io.WriteString(w, "\"")
		return err
	}
	if v.literal != nil {
		_, err := w.Write(v.literal)
		return err
	}
	_, err := io.Copy(w, v.reader())
	return err
}
func imageJSONLiteral(value any) (imageJSONValue, error) {
	raw, err := common.Marshal(value)
	return imageJSONValue{literal: raw}, err
}
func (v imageJSONValue) text() (string, error) {
	var s string
	err := common.DecodeJson(v.reader(), &s)
	return s, err
}

// Preserve the old DTO semantics: null and the empty string mean no image;
// other JSON types must not become a successful b64_json response.
func (v imageJSONValue) hasImageString() (bool, error) {
	if v.size == 4 {
		var marker [4]byte
		if _, err := v.source.ReadAt(marker[:], v.start); err != nil {
			return false, err
		}
		if string(marker[:]) == "null" {
			return false, nil
		}
	}
	if _, err := v.base64Reader(); err != nil {
		return false, err
	}
	return v.size > 2, nil
}

type imageJSONCursor struct {
	reader *bufio.Reader
	source io.ReaderAt
	pos    int64
}

func newImageJSONCursor(v imageJSONValue) *imageJSONCursor {
	return &imageJSONCursor{reader: bufio.NewReaderSize(v.reader(), 64<<10), source: v.source, pos: v.start}
}
func (p *imageJSONCursor) read() (byte, error) {
	b, err := p.reader.ReadByte()
	if err == nil {
		p.pos++
	}
	return b, err
}
func (p *imageJSONCursor) peek() (byte, error) {
	b, err := p.reader.Peek(1)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}
func (p *imageJSONCursor) space() error {
	for {
		b, err := p.peek()
		if err != nil {
			return err
		}
		if b != ' ' && b != '\n' && b != '\r' && b != '\t' {
			return nil
		}
		_, _ = p.read()
	}
}
func (p *imageJSONCursor) expect(want byte) error {
	if err := p.space(); err != nil {
		return err
	}
	b, err := p.read()
	if err != nil {
		return err
	}
	if b != want {
		return errors.New("invalid image JSON structure")
	}
	return nil
}
func (p *imageJSONCursor) value() (imageJSONValue, error) {
	if err := p.space(); err != nil {
		return imageJSONValue{}, err
	}
	start := p.pos
	depth, quoted, escaped := 0, false, false
	for {
		b, err := p.peek()
		if err == io.EOF && !quoted && depth == 0 && p.pos > start {
			break
		}
		if err != nil {
			return imageJSONValue{}, err
		}
		if !quoted && depth == 0 && (b == ',' || b == '}' || b == ']' || b == ' ' || b == '\n' || b == '\r' || b == '\t') {
			break
		}
		_, _ = p.read()
		if quoted {
			if escaped {
				escaped = false
			} else if b == '\\' {
				escaped = true
			} else if b == '"' {
				quoted = false
				if depth == 0 {
					break
				}
			}
		} else {
			switch b {
			case '"':
				quoted = true
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					return imageJSONValue{source: p.source, start: start, size: p.pos - start}, nil
				}
			}
		}
	}
	if p.pos == start || quoted || depth != 0 {
		return imageJSONValue{}, errors.New("invalid image JSON value")
	}
	return imageJSONValue{source: p.source, start: start, size: p.pos - start}, nil
}
func (v imageJSONValue) object() (map[string]imageJSONValue, error) {
	p := newImageJSONCursor(v)
	if err := p.expect('{'); err != nil {
		return nil, err
	}
	fields := make(map[string]imageJSONValue)
	for {
		if err := p.space(); err != nil {
			return nil, err
		}
		b, _ := p.peek()
		if b == '}' {
			_, _ = p.read()
			return fields, nil
		}
		key, err := p.value()
		if err != nil {
			return nil, err
		}
		name, err := key.text()
		if err != nil {
			return nil, err
		}
		if err := p.expect(':'); err != nil {
			return nil, err
		}
		value, err := p.value()
		if err != nil {
			return nil, err
		}
		fields[name] = value
		if err := p.space(); err != nil {
			return nil, err
		}
		b, _ = p.peek()
		if b == '}' {
			continue
		}
		if err := p.expect(','); err != nil {
			return nil, err
		}
	}
}
func (v imageJSONValue) array() ([]imageJSONValue, error) {
	p := newImageJSONCursor(v)
	if err := p.expect('['); err != nil {
		return nil, err
	}
	var values []imageJSONValue
	for {
		if err := p.space(); err != nil {
			return nil, err
		}
		b, _ := p.peek()
		if b == ']' {
			_, _ = p.read()
			return values, nil
		}
		value, err := p.value()
		if err != nil {
			return nil, err
		}
		values = append(values, value)
		if err := p.space(); err != nil {
			return nil, err
		}
		b, _ = p.peek()
		if b == ']' {
			continue
		}
		if err := p.expect(','); err != nil {
			return nil, err
		}
	}
}
func writeImageJSONObject(w io.Writer, fields map[string]imageJSONValue) error {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if _, err := io.WriteString(w, "{"); err != nil {
		return err
	}
	for i, key := range keys {
		if i > 0 {
			if _, err := io.WriteString(w, ","); err != nil {
				return err
			}
		}
		raw, err := common.Marshal(key)
		if err != nil {
			return err
		}
		if _, err := w.Write(raw); err != nil {
			return err
		}
		if _, err := io.WriteString(w, ":"); err != nil {
			return err
		}
		if err := fields[key].write(w); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "}")
	return err
}

// Base64 strings may contain JSON escapes (notably \/ and \u002b). Decode
// those a character at a time; never materialize the encoded image as a string.
type imageBase64JSONReader struct{ reader *bufio.Reader }

func (v imageJSONValue) base64Reader() (io.Reader, error) {
	var ends [1]byte
	if v.size < 2 {
		return nil, errors.New("invalid image base64")
	}
	if _, err := v.source.ReadAt(ends[:], v.start); err != nil || ends[0] != '"' {
		return nil, errors.New("invalid image base64")
	}
	if _, err := v.source.ReadAt(ends[:], v.start+v.size-1); err != nil || ends[0] != '"' {
		return nil, errors.New("invalid image base64")
	}
	return &imageBase64JSONReader{bufio.NewReaderSize(io.NewSectionReader(v.source, v.start+1, v.size-2), 64<<10)}, nil
}
func (r *imageBase64JSONReader) Read(dst []byte) (int, error) {
	for i := range dst {
		b, err := r.reader.ReadByte()
		if err != nil {
			return i, err
		}
		if b == '\\' {
			next, err := r.reader.ReadByte()
			if err != nil {
				return i, err
			}
			raw := []byte{'"', '\\', next}
			if next == 'u' {
				var hex [4]byte
				if _, err := io.ReadFull(r.reader, hex[:]); err != nil {
					return i, err
				}
				raw = append(raw, hex[:]...)
			}
			raw = append(raw, '"')
			var value string
			if err := common.Unmarshal(raw, &value); err != nil || len(value) != 1 {
				return i, errors.New("invalid image base64 escape")
			}
			b = value[0]
		}
		dst[i] = b
	}
	return len(dst), nil
}
