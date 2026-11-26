package agentactivity

import "strings"

// Tool is the small, provider-independent vocabulary used by the dock.
type Tool uint8

const (
	Think Tool = iota
	Read
	Edit
	Command
	LongCommand
)

// ClassifyTool projects only the tool category; names or arguments never
// leave the local activity reader.
func ClassifyTool(name string) Tool {
	name = strings.ToLower(name)
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		name = name[i+1:]
	}
	switch name {
	case "read", "glob", "grep", "webfetch", "websearch", "read_file", "view_file", "view_file_outline", "view_code_item", "grep_search", "find_by_name", "list_dir", "read_url_content", "search_web":
		return Read
	case "edit", "write", "multiedit", "notebookedit", "write_file", "edit_file", "apply_patch", "replace_file_content", "multi_replace_file_content", "write_to_file", "search_replace":
		return Edit
	case "bash", "shell", "exec_command", "write_stdin", "run_command", "send_command_input", "command_status", "process_start", "process_write", "process_poll":
		return Command
	}
	return Think
}
