package billingexpr

import (
	"bytes"
	"compress/gzip"
	"io"

	"github.com/QuantumNous/new-api/common"
)

// CalculationArchive is a lossless encoding for one batch's initial per-line
// evidence. It does not share data across jobs or require an evaluator to read.
// The 10,000-line fixture produces >20 MB of repetitive uncompressed JSON.
type CalculationArchive map[string]*Calculation

func (a CalculationArchive) MarshalJSON() ([]byte, error) {
	plain, err := common.Marshal(map[string]*Calculation(a))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	writer := gzip.NewWriter(&out)
	if _, err = writer.Write(plain); err != nil {
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	return common.Marshal(out.Bytes())
}

func (a *CalculationArchive) UnmarshalJSON(data []byte) error {
	var encoded []byte
	if err := common.Unmarshal(data, &encoded); err != nil {
		return err
	}
	reader, err := gzip.NewReader(bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	defer reader.Close()
	plain, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	var rows map[string]*Calculation
	if err = common.Unmarshal(plain, &rows); err != nil {
		return err
	}
	*a = rows
	return nil
}
