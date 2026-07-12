package toml

import (
	"fmt"
	"strings"

	"github.com/getsops/sops/v3"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable/edit"
)

type emitEntry struct {
	leading  []sops.Comment
	key      string
	value    any
	trailing []sops.Comment
}

type documentEmitter struct {
	lines []string
}

// EmitPlainFile emits one SOPS tree branch as a canonical TOML document.
func (store *Store) EmitPlainFile(branches sops.TreeBranches) ([]byte, error) {
	if len(branches) == 0 {
		return []byte{}, nil
	}
	if len(branches) != 1 {
		return nil, fmt.Errorf("TOML cannot represent %d documents in one file", len(branches))
	}
	emitter := &documentEmitter{}
	if err := emitter.emitRoot(branches[0]); err != nil {
		return nil, fmt.Errorf("could not marshal TOML data: %w", err)
	}
	out := emitter.bytes()
	var validated map[string]any
	if err := toml.Unmarshal(out, &validated); err != nil {
		return nil, fmt.Errorf("generated invalid TOML: %w", err)
	}
	return out, nil
}

// EmitValue emits a standalone TOML value. Tree branches are normalized to
// multiline inline tables because a table section is not a standalone value.
func (store *Store) EmitValue(value interface{}) ([]byte, error) {
	rendered, err := renderValue(value)
	if err != nil {
		return nil, fmt.Errorf("could not marshal TOML value: %w", err)
	}
	encoded := "value = " + rendered
	var validated map[string]any
	if err := toml.Unmarshal([]byte(encoded), &validated); err != nil {
		return nil, fmt.Errorf("generated invalid TOML value: %w", err)
	}
	return []byte(rendered), nil
}

func (emitter *documentEmitter) emitRoot(branch sops.TreeBranch) error {
	entries, finalComments, err := collectEntries(branch)
	if err != nil {
		return err
	}
	direct, sections := partitionEntries(entries)
	for _, entry := range direct {
		if err := emitter.emitKeyValue(entry); err != nil {
			return err
		}
	}
	for _, entry := range sections {
		if err := emitter.emitSection(nil, entry); err != nil {
			return err
		}
	}
	emitter.writeComments(finalComments)
	return nil
}

func (emitter *documentEmitter) emitKeyValue(entry emitEntry) error {
	emitter.writeComments(entry.leading)
	key, err := renderKey(entry.key)
	if err != nil {
		return err
	}
	value, err := renderValue(entry.value)
	if err != nil {
		return fmt.Errorf("key %q: %w", entry.key, err)
	}
	lines := combineKeyValue(key, value, "", false)
	lines = appendTrailingComments(lines, entry.trailing, "")
	emitter.lines = append(emitter.lines, lines...)
	return nil
}

func (emitter *documentEmitter) emitSection(parentPath []string, entry emitEntry) error {
	path := appendPath(parentPath, entry.key)
	switch value := entry.value.(type) {
	case sops.TreeBranch:
		return emitter.emitTable(path, value, entry.leading, entry.trailing, false)
	case []any:
		if !isArrayOfTables(value) {
			return fmt.Errorf("key %q is not a table section", entry.key)
		}
		return emitter.emitArrayOfTables(path, value, entry.leading, entry.trailing)
	default:
		return fmt.Errorf("key %q has unsupported section value %T", entry.key, entry.value)
	}
}

func (emitter *documentEmitter) emitTable(path []string, branch sops.TreeBranch, leading, outerTrailing []sops.Comment, arrayTable bool) error {
	branch, headerComment := takeHeaderComment(branch)
	entries, finalComments, err := collectEntries(branch)
	if err != nil {
		return err
	}
	direct, sections := partitionEntries(entries)
	headerComments := make([]sops.Comment, 0, 1+len(outerTrailing))
	if headerComment != nil {
		headerComments = append(headerComments, *headerComment)
	}
	headerComments = append(headerComments, outerTrailing...)
	needsHeader := arrayTable || len(headerComments) > 0 || len(direct) > 0 || len(finalComments) > 0 || len(sections) == 0

	if needsHeader {
		emitter.ensureSectionBreak()
		emitter.writeComments(leading)
		header, err := renderKeyPath(path)
		if err != nil {
			return err
		}
		if arrayTable {
			header = "[[" + header + "]]"
		} else {
			header = "[" + header + "]"
		}
		headerLines := appendTrailingComments([]string{header}, headerComments, "")
		emitter.lines = append(emitter.lines, headerLines...)
		for _, entry := range direct {
			if err := emitter.emitKeyValue(entry); err != nil {
				return err
			}
		}
		emitter.writeComments(finalComments)
	} else if len(sections) > 0 {
		sections[0].leading = append(append([]sops.Comment{}, leading...), sections[0].leading...)
	}

	for _, entry := range sections {
		if err := emitter.emitSection(path, entry); err != nil {
			return err
		}
	}
	return nil
}

func (emitter *documentEmitter) emitArrayOfTables(path []string, values []any, leading, trailing []sops.Comment) error {
	pending := append([]sops.Comment{}, leading...)
	emitted := false
	for _, value := range values {
		switch value := value.(type) {
		case sops.Comment:
			pending = append(pending, value)
		case string:
			comment, ok := encryptedComment(value)
			if !ok {
				return fmt.Errorf("array of tables %q contains %T", strings.Join(path, "."), value)
			}
			pending = append(pending, comment)
		case sops.TreeBranch:
			if err := emitter.emitTable(path, value, pending, nil, true); err != nil {
				return err
			}
			pending = pending[:0]
			emitted = true
		default:
			return fmt.Errorf("array of tables %q contains %T", strings.Join(path, "."), value)
		}
	}
	if !emitted {
		return fmt.Errorf("array of tables %q has no table elements", strings.Join(path, "."))
	}
	emitter.writeComments(pending)
	emitter.writeComments(trailing)
	return nil
}

func (emitter *documentEmitter) ensureSectionBreak() {
	if len(emitter.lines) == 0 || emitter.lines[len(emitter.lines)-1] == "" {
		return
	}
	emitter.lines = append(emitter.lines, "")
}

func (emitter *documentEmitter) writeComments(comments []sops.Comment) {
	for _, comment := range comments {
		emitter.lines = append(emitter.lines, renderCommentLines(comment.Value, "")...)
	}
}

func (emitter *documentEmitter) bytes() []byte {
	if len(emitter.lines) == 0 {
		return []byte{}
	}
	return []byte(strings.Join(emitter.lines, "\n") + "\n")
}

func collectEntries(branch sops.TreeBranch) ([]emitEntry, []sops.Comment, error) {
	var entries []emitEntry
	var pending []sops.Comment
	canAttachInline := false
	for _, item := range branch {
		if comment, ok := item.Key.(sops.Comment); ok {
			if comment.Inline && canAttachInline && len(entries) > 0 {
				entries[len(entries)-1].trailing = append(entries[len(entries)-1].trailing, comment)
				canAttachInline = false
			} else {
				pending = append(pending, comment)
				canAttachInline = false
			}
			continue
		}
		key, ok := item.Key.(string)
		if !ok {
			return nil, nil, fmt.Errorf("unsupported TOML key type %T", item.Key)
		}
		entries = append(entries, emitEntry{leading: pending, key: key, value: item.Value})
		pending = nil
		canAttachInline = true
	}
	return entries, pending, nil
}

func takeHeaderComment(branch sops.TreeBranch) (sops.TreeBranch, *sops.Comment) {
	if len(branch) == 0 {
		return branch, nil
	}
	comment, ok := branch[0].Key.(sops.Comment)
	if !ok || !comment.Inline {
		return branch, nil
	}
	return branch[1:], &comment
}

func partitionEntries(entries []emitEntry) (direct, sections []emitEntry) {
	for _, entry := range entries {
		if isSection(entry.value) {
			sections = append(sections, entry)
		} else {
			direct = append(direct, entry)
		}
	}
	return direct, sections
}

func isSection(value any) bool {
	switch value := value.(type) {
	case sops.TreeBranch:
		return true
	case []any:
		return isArrayOfTables(value)
	default:
		return false
	}
}

func isArrayOfTables(values []any) bool {
	hasTable := false
	for _, value := range values {
		switch value := value.(type) {
		case sops.Comment:
		case sops.TreeBranch:
			hasTable = true
		case string:
			if _, ok := encryptedComment(value); !ok {
				return false
			}
		default:
			return false
		}
	}
	return hasTable
}

func encryptedComment(value string) (sops.Comment, bool) {
	if !strings.HasPrefix(value, "ENC[") {
		return sops.Comment{}, false
	}
	if strings.HasSuffix(value, ",type:comment]") {
		return sops.Comment{Value: value}, true
	}
	if strings.HasSuffix(value, ",type:comment_inline]") {
		return sops.Comment{Value: value, Inline: true}, true
	}
	return sops.Comment{}, false
}

func appendPath(path []string, key string) []string {
	result := make([]string, len(path)+1)
	copy(result, path)
	result[len(path)] = key
	return result
}

func renderKeyPath(path []string) (string, error) {
	parts := make([]string, len(path))
	for i, key := range path {
		encoded, err := renderKey(key)
		if err != nil {
			return "", err
		}
		parts[i] = encoded
	}
	return strings.Join(parts, "."), nil
}

func renderKey(key string) (string, error) {
	document, err := edit.Parse(nil)
	if err != nil {
		return "", err
	}
	if err := document.Set([]string{key}, false); err != nil {
		return "", err
	}
	line := strings.TrimSuffix(document.String(), "\n")
	const suffix = " = false"
	if !strings.HasSuffix(line, suffix) {
		return "", fmt.Errorf("go-toml returned an unexpected key encoding %q", line)
	}
	return strings.TrimSuffix(line, suffix), nil
}

func renderValue(value any) (string, error) {
	switch value := value.(type) {
	case sops.TreeBranch:
		return renderInlineTable(value)
	case []any:
		return renderArray(value)
	case sops.Comment:
		return "", fmt.Errorf("a comment cannot be encoded as a TOML value")
	case sops.TreeBranches:
		return "", fmt.Errorf("multiple tree branches cannot be encoded as one TOML value")
	default:
		return renderScalar(value)
	}
}

func renderScalar(value any) (string, error) {
	document, err := edit.Parse(nil)
	if err != nil {
		return "", err
	}
	if err := document.Set([]string{"value"}, value); err != nil {
		return "", err
	}
	line := strings.TrimSuffix(document.String(), "\n")
	const prefix = "value = "
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("go-toml returned an unexpected value encoding %q", line)
	}
	return strings.TrimPrefix(line, prefix), nil
}

func renderArray(values []any) (string, error) {
	lines := []string{"["}
	lastAttachable := 0
	indent := "  "
	for _, value := range values {
		if comment, ok := value.(sops.Comment); ok {
			if comment.Inline && lastAttachable >= 0 {
				lines = appendTrailingComment(lines, lastAttachable, comment, indent)
				lastAttachable = -1
			} else {
				lines = append(lines, renderCommentLines(comment.Value, indent)...)
				lastAttachable = -1
			}
			continue
		}
		rendered, err := renderValue(value)
		if err != nil {
			return "", err
		}
		valueLines := indentMultiline(rendered, indent)
		valueLines[len(valueLines)-1] += ","
		lines = append(lines, valueLines...)
		lastAttachable = len(lines) - 1
	}
	lines = append(lines, "]")
	return strings.Join(lines, "\n"), nil
}

func renderInlineTable(branch sops.TreeBranch) (string, error) {
	branch, headerComment := takeHeaderComment(branch)
	entries, finalComments, err := collectEntries(branch)
	if err != nil {
		return "", err
	}
	lines := []string{"{"}
	if headerComment != nil {
		lines = appendTrailingComment(lines, 0, *headerComment, "  ")
	}
	indent := "  "
	for _, entry := range entries {
		for _, comment := range entry.leading {
			lines = append(lines, renderCommentLines(comment.Value, indent)...)
		}
		key, err := renderKey(entry.key)
		if err != nil {
			return "", err
		}
		value, err := renderValue(entry.value)
		if err != nil {
			return "", fmt.Errorf("key %q: %w", entry.key, err)
		}
		valueLines := combineKeyValue(key, value, indent, true)
		valueLines = appendTrailingComments(valueLines, entry.trailing, indent)
		lines = append(lines, valueLines...)
	}
	for _, comment := range finalComments {
		lines = append(lines, renderCommentLines(comment.Value, indent)...)
	}
	lines = append(lines, "}")
	return strings.Join(lines, "\n"), nil
}

func combineKeyValue(key, value, indent string, comma bool) []string {
	valueLines := strings.Split(value, "\n")
	result := make([]string, len(valueLines))
	result[0] = indent + key + " = " + valueLines[0]
	for i := 1; i < len(valueLines); i++ {
		result[i] = indent + valueLines[i]
	}
	if comma {
		result[len(result)-1] += ","
	}
	return result
}

func indentMultiline(value, indent string) []string {
	parts := strings.Split(value, "\n")
	for i := range parts {
		parts[i] = indent + parts[i]
	}
	return parts
}

func appendTrailingComments(lines []string, comments []sops.Comment, indent string) []string {
	for i, comment := range comments {
		if i == 0 {
			lines = appendTrailingComment(lines, len(lines)-1, comment, indent)
		} else {
			lines = append(lines, renderCommentLines(comment.Value, indent)...)
		}
	}
	return lines
}

func appendTrailingComment(lines []string, lineIndex int, comment sops.Comment, indent string) []string {
	commentLines := splitComment(comment.Value)
	lines[lineIndex] += " #"
	if commentLines[0] != "" {
		lines[lineIndex] += " " + commentLines[0]
	}
	for _, line := range commentLines[1:] {
		lines = append(lines, renderCommentLine(line, indent))
	}
	return lines
}

func renderCommentLines(value, indent string) []string {
	parts := splitComment(value)
	result := make([]string, len(parts))
	for i, part := range parts {
		result[i] = renderCommentLine(part, indent)
	}
	return result
}

func renderCommentLine(value, indent string) string {
	if value == "" {
		return indent + "#"
	}
	return indent + "# " + value
}

func splitComment(value string) []string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.Split(value, "\n")
}
