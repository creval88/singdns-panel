package webassets

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var inlineScriptRE = regexp.MustCompile(`(?is)<script(?:\s[^>]*)?>(.*?)</script>`)
var goTemplateExprRE = regexp.MustCompile(`{{[^{}]*}}`)

func TestInlineTemplateScriptsSyntax(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; skipping inline template JavaScript syntax checks")
	}

	entries, err := os.ReadDir("templates")
	if err != nil {
		t.Fatalf("read templates: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".html") {
			continue
		}
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join("templates", name))
			if err != nil {
				t.Fatalf("read template: %v", err)
			}

			matches := inlineScriptRE.FindAllSubmatch(body, -1)
			if len(matches) == 0 {
				return
			}

			var scripts []string
			for _, match := range matches {
				if len(match) < 2 {
					continue
				}
				script := strings.TrimSpace(string(match[1]))
				if script == "" {
					continue
				}
				scripts = append(scripts, goTemplateExprRE.ReplaceAllString(script, "null"))
			}
			if len(scripts) == 0 {
				return
			}

			tmp := filepath.Join(t.TempDir(), name+".js")
			if err := os.WriteFile(tmp, []byte(strings.Join(scripts, "\n;\n")), 0600); err != nil {
				t.Fatalf("write temp script: %v", err)
			}

			cmd := exec.Command(node, "--check", tmp)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("node --check failed:\n%s", out)
			}
		})
	}
}
