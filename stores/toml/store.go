package toml //import "github.com/YewFence/sops/v3/stores/toml"

import (
	"bytes"
	"fmt"

	"github.com/YewFence/sops/v3"
	"github.com/YewFence/sops/v3/config"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

// Store handles storage of TOML data.
type Store struct {
	config config.TOMLStoreConfig
}

func NewStore(c *config.TOMLStoreConfig) *Store {
	return &Store{config: *c}
}

func (store *Store) Name() string {
	return "toml"
}

// LoadPlainFile loads plaintext TOML into an ordered SOPS tree.
func (store *Store) LoadPlainFile(in []byte) (sops.TreeBranches, error) {
	branch, err := parseTOML(in)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal TOML data: %w", err)
	}
	return sops.TreeBranches{branch}, nil
}

type branchRef struct {
	items []sops.TreeItem
}

func finalizeBranch(branch *branchRef) sops.TreeBranch {
	if branch == nil {
		return nil
	}
	result := make(sops.TreeBranch, len(branch.items))
	for i, item := range branch.items {
		result[i] = item
		result[i].Value = finalizeValue(item.Value)
	}
	return result
}

func finalizeValue(value any) any {
	switch value := value.(type) {
	case *branchRef:
		return finalizeBranch(value)
	case []any:
		result := make([]any, len(value))
		for i, item := range value {
			result[i] = finalizeValue(item)
		}
		return result
	default:
		return value
	}
}

type parseState struct {
	root  *branchRef
	scope *branchRef
}

func newParseState() *parseState {
	root := &branchRef{}
	return &parseState{root: root, scope: root}
}

func branchForItem(item sops.TreeItem) *branchRef {
	switch value := item.Value.(type) {
	case *branchRef:
		return value
	case []any:
		if len(value) > 0 {
			branch, _ := value[len(value)-1].(*branchRef)
			return branch
		}
	}
	return nil
}

func (state *parseState) navigateTo(branch *branchRef, key string) *branchRef {
	for i := len(branch.items) - 1; i >= 0; i-- {
		itemKey, ok := branch.items[i].Key.(string)
		if ok && itemKey == key {
			if child := branchForItem(branch.items[i]); child != nil {
				return child
			}
			break
		}
	}
	child := &branchRef{}
	branch.items = append(branch.items, sops.TreeItem{Key: key, Value: child})
	return child
}

// navigateForSection starts a new structural entry when another table section
// has been emitted since the last matching entry. This keeps interleaved table
// sections in source order, for example [a.b], [c], [a.d].
func (state *parseState) navigateForSection(branch *branchRef, key string) *branchRef {
	for i := len(branch.items) - 1; i >= 0; i-- {
		itemKey, ok := branch.items[i].Key.(string)
		if !ok || itemKey != key {
			continue
		}
		hasLaterSection := false
		for _, later := range branch.items[i+1:] {
			if _, ok := later.Key.(sops.Comment); ok {
				continue
			}
			if branchForItem(later) != nil {
				hasLaterSection = true
				break
			}
		}
		if !hasLaterSection {
			if child := branchForItem(branch.items[i]); child != nil {
				return child
			}
		}
		break
	}
	child := &branchRef{}
	branch.items = append(branch.items, sops.TreeItem{Key: key, Value: child})
	return child
}

func (state *parseState) targetForKey(path []string) *branchRef {
	target := state.scope
	for _, key := range path {
		target = state.navigateTo(target, key)
	}
	return target
}

func (state *parseState) parentForSection(path []string) *branchRef {
	parent := state.root
	for _, key := range path[:len(path)-1] {
		parent = state.navigateForSection(parent, key)
	}
	return parent
}

func (state *parseState) setTableScope(path []string) *branchRef {
	scope := state.root
	for _, key := range path {
		scope = state.navigateForSection(scope, key)
	}
	state.scope = scope
	return scope
}

func (state *parseState) setArrayTableScope(path []string, leadingComments []sops.Comment) *branchRef {
	parent := state.root
	for _, key := range path[:len(path)-1] {
		parent = state.navigateForSection(parent, key)
	}
	key := path[len(path)-1]
	for i := len(parent.items) - 1; i >= 0; i-- {
		itemKey, ok := parent.items[i].Key.(string)
		if !ok || itemKey != key {
			continue
		}
		if array, ok := parent.items[i].Value.([]any); ok {
			scope := &branchRef{}
			for _, comment := range leadingComments {
				array = append(array, comment)
			}
			parent.items[i].Value = append(array, scope)
			state.scope = scope
			return scope
		}
		break
	}
	appendComments(parent, leadingComments)
	scope := &branchRef{}
	parent.items = append(parent.items, sops.TreeItem{Key: key, Value: []any{scope}})
	state.scope = scope
	return scope
}

func appendComments(target *branchRef, comments []sops.Comment) {
	for _, comment := range comments {
		target.items = append(target.items, sops.TreeItem{Key: comment, Value: nil})
	}
}

func parseTOML(data []byte) (sops.TreeBranch, error) {
	var validated map[string]any
	if err := toml.Unmarshal(data, &validated); err != nil {
		return nil, err
	}

	var parser unstable.Parser
	parser.KeepComments = true
	parser.Reset(data)
	state := newParseState()
	var pendingComments []sops.Comment

	for parser.NextExpression() {
		node := parser.Expression()
		switch node.Kind {
		case unstable.Comment:
			pendingComments = appendCommentTree(pendingComments, data, node)
		case unstable.KeyValue:
			path, err := keyPath(node)
			if err != nil {
				return nil, err
			}
			value, err := nodeToValue(&parser, data, node.Value())
			if err != nil {
				return nil, err
			}
			trailingComments := commentsFromSiblings(data, node.Next())
			if node.Value().Kind == unstable.InlineTable {
				value, trailingComments = attachInlineTableHeaderComments(value.(sops.TreeBranch), trailingComments)
			}
			target := state.targetForKey(path[:len(path)-1])
			appendComments(target, pendingComments)
			pendingComments = pendingComments[:0]
			target.items = append(target.items, sops.TreeItem{Key: path[len(path)-1], Value: value})
			appendComments(target, trailingComments)
		case unstable.Table:
			path, err := keyPath(node)
			if err != nil {
				return nil, err
			}
			appendComments(state.parentForSection(path), pendingComments)
			pendingComments = pendingComments[:0]
			scope := state.setTableScope(path)
			appendComments(scope, commentsFromSiblings(data, node.Next()))
		case unstable.ArrayTable:
			path, err := keyPath(node)
			if err != nil {
				return nil, err
			}
			scope := state.setArrayTableScope(path, pendingComments)
			pendingComments = pendingComments[:0]
			appendComments(scope, commentsFromSiblings(data, node.Next()))
		}
	}
	if err := parser.Error(); err != nil {
		return nil, err
	}
	appendComments(state.scope, pendingComments)
	return finalizeBranch(state.root), nil
}

// Inline tables are emitted as table sections, whose header comments live inside the table branch.
func attachInlineTableHeaderComments(branch sops.TreeBranch, comments []sops.Comment) (sops.TreeBranch, []sops.Comment) {
	var headerComments []sops.Comment
	var remainingComments []sops.Comment
	for _, comment := range comments {
		if comment.Inline {
			headerComments = append(headerComments, comment)
		} else {
			remainingComments = append(remainingComments, comment)
		}
	}
	if len(headerComments) == 0 {
		return branch, remainingComments
	}
	result := make(sops.TreeBranch, 0, len(headerComments)+len(branch))
	for _, comment := range headerComments {
		result = append(result, sops.TreeItem{Key: comment})
	}
	return append(result, branch...), remainingComments
}

func keyPath(node *unstable.Node) ([]string, error) {
	var path []string
	iterator := node.Key()
	for iterator.Next() {
		path = append(path, string(iterator.Node().Data))
	}
	if len(path) == 0 {
		return nil, fmt.Errorf("TOML %s has an empty key path", node.Kind)
	}
	return path, nil
}

func nodeToValue(parser *unstable.Parser, data []byte, node *unstable.Node) (any, error) {
	switch node.Kind {
	case unstable.Array:
		return nodeToArray(parser, data, node)
	case unstable.InlineTable:
		return nodeToInlineTable(parser, data, node)
	case unstable.String, unstable.Bool, unstable.Float, unstable.Integer,
		unstable.LocalDate, unstable.LocalTime, unstable.LocalDateTime, unstable.DateTime:
		return decodeScalar(parser.Raw(node.Raw))
	default:
		return nil, fmt.Errorf("unsupported TOML node kind %s", node.Kind)
	}
}

func decodeScalar(raw []byte) (any, error) {
	encoded := make([]byte, 0, len(raw)+8)
	encoded = append(encoded, "value = "...)
	encoded = append(encoded, raw...)
	var decoded map[string]any
	if err := toml.Unmarshal(encoded, &decoded); err != nil {
		return nil, err
	}
	return decoded["value"], nil
}

func nodeToArray(parser *unstable.Parser, data []byte, node *unstable.Node) ([]any, error) {
	result := make([]any, 0)
	iterator := node.Children()
	for iterator.Next() {
		child := iterator.Node()
		if child.Kind == unstable.Comment {
			comments := appendCommentTree(nil, data, child)
			for _, comment := range comments {
				result = append(result, comment)
			}
			continue
		}
		value, err := nodeToValue(parser, data, child)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func nodeToInlineTable(parser *unstable.Parser, data []byte, node *unstable.Node) (sops.TreeBranch, error) {
	state := newParseState()
	lastTarget := state.root
	var pendingComments []sops.Comment
	iterator := node.Children()
	for iterator.Next() {
		child := iterator.Node()
		if child.Kind == unstable.Comment {
			comments := appendCommentTree(nil, data, child)
			for _, comment := range comments {
				if comment.Inline {
					appendComments(lastTarget, []sops.Comment{comment})
				} else {
					pendingComments = append(pendingComments, comment)
				}
			}
			continue
		}
		if child.Kind != unstable.KeyValue {
			return nil, fmt.Errorf("unexpected %s in TOML inline table", child.Kind)
		}
		path, err := keyPath(child)
		if err != nil {
			return nil, err
		}
		value, err := nodeToValue(parser, data, child.Value())
		if err != nil {
			return nil, err
		}
		target := state.targetForKey(path[:len(path)-1])
		appendComments(target, pendingComments)
		pendingComments = pendingComments[:0]
		target.items = append(target.items, sops.TreeItem{Key: path[len(path)-1], Value: value})
		lastTarget = target
	}
	appendComments(state.root, pendingComments)
	return finalizeBranch(state.root), nil
}

func commentsFromSiblings(data []byte, node *unstable.Node) []sops.Comment {
	var comments []sops.Comment
	for current := node; current != nil; current = current.Next() {
		if current.Kind == unstable.Comment {
			comments = appendCommentTree(comments, data, current)
		}
	}
	return comments
}

func appendCommentTree(comments []sops.Comment, data []byte, node *unstable.Node) []sops.Comment {
	comments = append(comments, commentFromNode(data, node))
	children := node.Children()
	for children.Next() {
		child := children.Node()
		if child.Kind == unstable.Comment {
			comments = appendCommentTree(comments, data, child)
		}
	}
	return comments
}

func commentFromNode(data []byte, node *unstable.Node) sops.Comment {
	value := bytes.TrimPrefix(node.Data, []byte("#"))
	value = bytes.TrimPrefix(value, []byte(" "))
	return sops.Comment{
		Value:  string(value),
		Inline: isInlineComment(data, int(node.Raw.Offset)),
	}
}

func isInlineComment(data []byte, offset int) bool {
	if offset <= 0 || offset > len(data) {
		return false
	}
	lineStart := bytes.LastIndexByte(data[:offset], '\n') + 1
	return len(bytes.TrimSpace(data[lineStart:offset])) > 0
}
