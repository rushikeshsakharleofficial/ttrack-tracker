package ansible

import (
	"strings"
	"testing"

	"ttrack/internal/config"
)

const sampleRun = `
{"type":"run","id":"20260527T120000-1234","playbook":"deploy.yml","user":"alice","started":1748337600,"controller":"ctrl.example.com"}
{"type":"play","name":"Install web server"}
{"type":"task","play":"Install web server","name":"install nginx","module":"ansible.builtin.dnf","host":"web1","status":"changed","rc":0,"t":1748337601,"stdout":"Installed: nginx\n","stderr":""}
{"type":"task","play":"Install web server","name":"install nginx","module":"ansible.builtin.dnf","host":"web2","status":"ok","rc":0,"t":1748337602}
{"type":"task","play":"Install web server","name":"fail intentionally","module":"ansible.builtin.command","host":"web1","status":"failed","rc":1,"t":1748337603,"stdout":"","stderr":"command not found\n"}
{"type":"task","play":"Install web server","name":"secret task","module":"ansible.builtin.shell","host":"web1","status":"ok","rc":0,"t":1748337604,"stdout":"<censored: no_log>","stderr":"<censored: no_log>"}
{"type":"task","play":"Install web server","name":"skip me","module":"ansible.builtin.debug","host":"web2","status":"skipped","t":1748337605}
{"type":"stats","host":"web1","ok":1,"changed":1,"failed":1,"unreachable":0,"skipped":0}
{"type":"stats","host":"web2","ok":2,"changed":0,"failed":0,"unreachable":0,"skipped":1}
`

func TestParseRun_basic(t *testing.T) {
	run, err := ParseRun(strings.NewReader(sampleRun))
	if err != nil {
		t.Fatalf("ParseRun error: %v", err)
	}

	if run.ID != "20260527T120000-1234" {
		t.Errorf("ID = %q, want 20260527T120000-1234", run.ID)
	}
	if run.Playbook != "deploy.yml" {
		t.Errorf("Playbook = %q, want deploy.yml", run.Playbook)
	}
	if run.User != "alice" {
		t.Errorf("User = %q, want alice", run.User)
	}
	if run.Controller != "ctrl.example.com" {
		t.Errorf("Controller = %q", run.Controller)
	}
}

func TestParseRun_plays(t *testing.T) {
	run, _ := ParseRun(strings.NewReader(sampleRun))
	if len(run.Plays) != 1 || run.Plays[0] != "Install web server" {
		t.Errorf("Plays = %v, want [Install web server]", run.Plays)
	}
}

func TestParseRun_tasks(t *testing.T) {
	run, _ := ParseRun(strings.NewReader(sampleRun))
	if len(run.Tasks) != 5 {
		t.Errorf("len(Tasks) = %d, want 5", len(run.Tasks))
	}
	// First task: changed
	if run.Tasks[0].Status != "changed" {
		t.Errorf("Tasks[0].Status = %q, want changed", run.Tasks[0].Status)
	}
	if run.Tasks[0].Host != "web1" {
		t.Errorf("Tasks[0].Host = %q, want web1", run.Tasks[0].Host)
	}
	if run.Tasks[0].Stdout != "Installed: nginx\n" {
		t.Errorf("Tasks[0].Stdout = %q", run.Tasks[0].Stdout)
	}
	// Failed task: has stderr
	if run.Tasks[2].Status != "failed" {
		t.Errorf("Tasks[2].Status = %q, want failed", run.Tasks[2].Status)
	}
	if run.Tasks[2].RC != 1 {
		t.Errorf("Tasks[2].RC = %d, want 1", run.Tasks[2].RC)
	}
	if !strings.Contains(run.Tasks[2].Stderr, "command not found") {
		t.Errorf("Tasks[2].Stderr = %q, want 'command not found'", run.Tasks[2].Stderr)
	}
}

func TestParseRun_noLog_censored(t *testing.T) {
	run, _ := ParseRun(strings.NewReader(sampleRun))
	// The no_log task has stdout/stderr set to "<censored: no_log>" by the plugin.
	// ParseRun must preserve it as-is (censoring is the plugin's responsibility).
	secret := run.Tasks[3]
	if secret.Name != "secret task" {
		t.Errorf("unexpected task name %q", secret.Name)
	}
	if !strings.Contains(secret.Stdout, "censored") {
		t.Errorf("Stdout should be censored, got %q", secret.Stdout)
	}
}

func TestParseRun_stats(t *testing.T) {
	run, _ := ParseRun(strings.NewReader(sampleRun))
	if run.TotalOK != 3 {
		t.Errorf("TotalOK = %d, want 3", run.TotalOK)
	}
	if run.TotalChanged != 1 {
		t.Errorf("TotalChanged = %d, want 1", run.TotalChanged)
	}
	if run.TotalFailed != 1 {
		t.Errorf("TotalFailed = %d, want 1", run.TotalFailed)
	}
	if run.TotalSkipped != 1 {
		t.Errorf("TotalSkipped = %d, want 1", run.TotalSkipped)
	}
	if _, ok := run.Stats["web1"]; !ok {
		t.Error("no stats for web1")
	}
}

func TestParseRun_hosts_sorted(t *testing.T) {
	run, _ := ParseRun(strings.NewReader(sampleRun))
	if len(run.Hosts) != 2 || run.Hosts[0] != "web1" || run.Hosts[1] != "web2" {
		t.Errorf("Hosts = %v, want [web1 web2]", run.Hosts)
	}
}

func TestParseRun_empty(t *testing.T) {
	run, err := ParseRun(strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(run.Tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(run.Tasks))
	}
}

func TestParseRun_garbage_lines_skipped(t *testing.T) {
	input := `not json at all
{"type":"run","id":"abc-1","playbook":"x.yml","user":"u","started":1000,"controller":"h"}
another garbage line
{"type":"stats","host":"h1","ok":2}
`
	run, err := ParseRun(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run.ID != "abc-1" {
		t.Errorf("ID = %q, want abc-1", run.ID)
	}
	if run.TotalOK != 2 {
		t.Errorf("TotalOK = %d, want 2", run.TotalOK)
	}
}

func TestTruncate(t *testing.T) {
	short := "hello"
	if truncate(short) != short {
		t.Errorf("short string should not be truncated")
	}
	long := strings.Repeat("x", config.Load().AnsibleOutputCap+100)
	got := truncate(long)
	if len(got) <= config.Load().AnsibleOutputCap {
		// OK — truncated
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("truncated string should contain 'truncated' marker")
	}
}

func TestValidAnsibleRunID_ingest(t *testing.T) {
	good := []string{
		"20260527T120000-1234",
		"20260527T120000-99999",
		"abc-123_T",
	}
	bad := []string{
		"",
		"ab",
		"../../etc/passwd",
		"run/id",
		strings.Repeat("a", 65),
		"has space",
	}
	// validRunID is unexported but tested via package-level access.
	for _, id := range good {
		if !validRunID(id) {
			t.Errorf("expected valid: %q", id)
		}
	}
	for _, id := range bad {
		if validRunID(id) {
			t.Errorf("expected invalid: %q", id)
		}
	}
}
