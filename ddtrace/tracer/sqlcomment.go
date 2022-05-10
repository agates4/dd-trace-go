package tracer

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// SQLCommentCarrier holds tags to be serialized as a SQL Comment
type SQLCommentCarrier struct {
	tags map[string]string
}

// Values for sql comment keys
const (
	SamplingPrioritySQLCommentKey   = "ddsp"
	TraceIDSQLCommentKey            = "ddtid"
	SpanIDSQLCommentKey             = "ddsid"
	ServiceNameSQLCommentKey        = "ddsn"
	ServiceVersionSQLCommentKey     = "ddsv"
	ServiceEnvironmentSQLCommentKey = "dde"
)

// Set implements TextMapWriter.
func (c *SQLCommentCarrier) Set(key, val string) {
	if c.tags == nil {
		c.tags = make(map[string]string)
	}

	c.tags[key] = val
}

func commentWithTags(tags map[string]string) (comment string) {
	if len(tags) == 0 {
		return ""
	}

	serializedTags := make([]string, 0, len(tags))
	for k, v := range tags {
		serializedTags = append(serializedTags, serializeTag(k, v))
	}

	sort.Strings(serializedTags)
	comment = strings.Join(serializedTags, ",")
	return fmt.Sprintf("/*%s*/", comment)
}

// CommentedQuery returns the given query with the tags from the SQLCommentCarrier applied to it as a
// prepended SQL comment
func (c *SQLCommentCarrier) CommentedQuery(query string) (commented string) {
	comment := commentWithTags(c.tags)

	if comment == "" {
		return query
	}

	if query == "" {
		return comment
	}

	return fmt.Sprintf("%s %s", comment, query)
}

// ForeachKey implements TextMapReader.
func (c SQLCommentCarrier) ForeachKey(handler func(key, val string) error) error {
	for k, v := range c.tags {
		if err := handler(k, v); err != nil {
			return err
		}
	}
	return nil
}

func serializeTag(key string, value string) (serialized string) {
	sKey := serializeKey(key)
	sValue := serializeValue(value)

	return fmt.Sprintf("%s=%s", sKey, sValue)
}

func serializeKey(key string) (encoded string) {
	urlEncoded := url.PathEscape(key)
	escapedMeta := escapeMetaChars(urlEncoded)

	return escapedMeta
}

func serializeValue(value string) (encoded string) {
	urlEncoded := url.PathEscape(value)
	escapedMeta := escapeMetaChars(urlEncoded)
	escaped := escapeSQL(escapedMeta)

	return escaped
}

func escapeSQL(value string) (escaped string) {
	return fmt.Sprintf("'%s'", value)
}

func escapeMetaChars(value string) (escaped string) {
	return strings.ReplaceAll(value, "'", "\\'")
}
