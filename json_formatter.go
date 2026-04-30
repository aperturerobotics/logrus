package logrus

import (
	"bytes"
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

type fieldKey string

// FieldMap allows customization of the key names for default fields.
type FieldMap map[fieldKey]string

func (f FieldMap) resolve(key fieldKey) string {
	if k, ok := f[key]; ok {
		return k
	}

	return string(key)
}

// JSONFormatter formats logs into parsable json
type JSONFormatter struct {
	// TimestampFormat sets the format used for marshaling timestamps.
	// The format to use is the same than for time.Format or time.Parse from the standard
	// library.
	// The standard Library already provides a set of predefined format.
	TimestampFormat string

	// DisableTimestamp allows disabling automatic timestamps in output
	DisableTimestamp bool

	// DisableHTMLEscape allows disabling html escaping in output
	DisableHTMLEscape bool

	// DataKey allows users to put all the log entry parameters into a nested dictionary at a given key.
	DataKey string

	// FieldMap allows users to customize the names of keys for default fields.
	// As an example:
	// formatter := &JSONFormatter{
	//   	FieldMap: FieldMap{
	// 		 FieldKeyTime:  "@timestamp",
	// 		 FieldKeyLevel: "@level",
	// 		 FieldKeyMsg:   "@message",
	// 		 FieldKeyFunc:  "@caller",
	//    },
	// }
	FieldMap FieldMap

	// CallerPrettyfier can be set by the user to modify the content
	// of the function and file keys in the json data when ReportCaller is
	// activated. If any of the returned value is the empty string the
	// corresponding key will be removed from json fields.
	CallerPrettyfier func(*runtime.Frame) (function string, file string)

	// PrettyPrint will indent all json logs
	PrettyPrint bool
}

// Format renders a single log entry
func (f *JSONFormatter) Format(entry *Entry) ([]byte, error) {
	caller := entry.Caller
	data := make(Fields, len(entry.Data)+defaultFields)
	for k, v := range entry.Data {
		switch v := v.(type) {
		case error:
			// Otherwise errors are ignored by `encoding/json`
			// https://github.com/sirupsen/logrus/issues/137
			data[k] = v.Error()
		default:
			data[k] = v
		}
	}

	if f.DataKey != "" && len(entry.Data) > 0 {
		newData := make(Fields, defaultFields+1)
		newData[f.DataKey] = data
		data = newData
	}

	hasCaller := caller != nil
	prefixFieldClashes(data, f.FieldMap, hasCaller)

	timestampFormat := f.TimestampFormat
	if timestampFormat == "" {
		timestampFormat = defaultTimestampFormat
	}

	if entry.err != "" {
		data[f.FieldMap.resolve(FieldKeyLogrusError)] = entry.err
	}
	if !f.DisableTimestamp {
		data[f.FieldMap.resolve(FieldKeyTime)] = entry.Time.Format(timestampFormat)
	}
	data[f.FieldMap.resolve(FieldKeyMsg)] = entry.Message
	data[f.FieldMap.resolve(FieldKeyLevel)] = entry.Level.String()
	if caller != nil {
		var funcVal, fileVal string
		if f.CallerPrettyfier != nil {
			funcVal, fileVal = f.CallerPrettyfier(caller)
		} else {
			funcVal = caller.Function
			fileVal = caller.File + ":" + strconv.FormatInt(int64(caller.Line), 10)
		}
		if funcVal != "" {
			data[f.FieldMap.resolve(FieldKeyFunc)] = funcVal
		}
		if fileVal != "" {
			data[f.FieldMap.resolve(FieldKeyFile)] = fileVal
		}
	}

	b := entry.Buffer
	if b == nil {
		b = new(bytes.Buffer)
	}

	if err := f.appendJSONFields(b, data, ""); err != nil {
		return nil, fmt.Errorf("failed to marshal fields to JSON, %w", err)
	}
	b.WriteByte('\n')

	return b.Bytes(), nil
}

type jsonMarshaler interface {
	MarshalJSON() ([]byte, error)
}

func (f *JSONFormatter) appendJSONFields(b *bytes.Buffer, data Fields, prefix string) error {
	b.WriteByte('{')
	keys := make([]string, 0, len(data))
	for k, v := range data {
		keys = append(keys, k)
		_ = v
	}
	sort.Strings(keys)
	for i, k := range keys {
		v := data[k]
		if i != 0 {
			b.WriteByte(',')
		}
		if f.PrettyPrint {
			b.WriteByte('\n')
			b.WriteString(prefix)
			b.WriteString("  ")
		}
		f.appendJSONString(b, k)
		b.WriteByte(':')
		if f.PrettyPrint {
			b.WriteByte(' ')
		}
		if err := f.appendJSONValue(b, v, prefix+"  "); err != nil {
			return err
		}
	}
	if f.PrettyPrint && len(keys) != 0 {
		b.WriteByte('\n')
		b.WriteString(prefix)
	}
	b.WriteByte('}')
	return nil
}

func (f *JSONFormatter) appendJSONValue(b *bytes.Buffer, value any, prefix string) error {
	switch v := value.(type) {
	case nil:
		b.WriteString("null")
	case string:
		f.appendJSONString(b, v)
	case []byte:
		f.appendJSONString(b, string(v))
	case error:
		f.appendJSONString(b, v.Error())
	case jsonMarshaler:
		out, err := v.MarshalJSON()
		if err != nil {
			return err
		}
		b.Write(out)
	case bool:
		b.WriteString(strconv.FormatBool(v))
	case int:
		b.WriteString(strconv.FormatInt(int64(v), 10))
	case int8:
		b.WriteString(strconv.FormatInt(int64(v), 10))
	case int16:
		b.WriteString(strconv.FormatInt(int64(v), 10))
	case int32:
		b.WriteString(strconv.FormatInt(int64(v), 10))
	case int64:
		b.WriteString(strconv.FormatInt(v, 10))
	case uint:
		b.WriteString(strconv.FormatUint(uint64(v), 10))
	case uint8:
		b.WriteString(strconv.FormatUint(uint64(v), 10))
	case uint16:
		b.WriteString(strconv.FormatUint(uint64(v), 10))
	case uint32:
		b.WriteString(strconv.FormatUint(uint64(v), 10))
	case uint64:
		b.WriteString(strconv.FormatUint(v, 10))
	case uintptr:
		b.WriteString(strconv.FormatUint(uint64(v), 10))
	case float32:
		b.WriteString(strconv.FormatFloat(float64(v), 'g', -1, 32))
	case float64:
		b.WriteString(strconv.FormatFloat(v, 'g', -1, 64))
	case Fields:
		return f.appendJSONFields(b, v, prefix)
	default:
		f.appendJSONString(b, fmt.Sprint(v))
	}
	return nil
}

func (f *JSONFormatter) appendJSONString(b *bytes.Buffer, s string) {
	var tmp [128]byte
	out := string(strconv.AppendQuote(tmp[:0], s))
	if !f.DisableHTMLEscape {
		out = strings.ReplaceAll(out, "&", `\u0026`)
		out = strings.ReplaceAll(out, "<", `\u003c`)
		out = strings.ReplaceAll(out, ">", `\u003e`)
	}
	b.WriteString(out)
}
