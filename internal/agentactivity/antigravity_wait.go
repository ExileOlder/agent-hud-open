package agentactivity

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

type AntigravityWait struct {
	Account, Cascade, Trajectory, Execution string
	Step                                    uint32
	Permission                              bool
}

type antigravityDaemon struct {
	PID       uint32 `json:"pid"`
	HTTPSPort uint16 `json:"httpsPort"`
	Token     string `json:"csrfToken"`
}

type daemonAddress struct {
	pid  uint32
	port uint16
}

const antigravityServicePath = "/exa.language_server_pb.LanguageServerService/"
const antigravityReadPath = antigravityServicePath + "GetAllCascadeTrajectories"
const antigravityIdentityPath = antigravityServicePath + "GetUserStatus"
const antigravityMetadata = `{"metadata":{"ideName":"antigravity","extensionName":"antigravity","locale":"en","ideVersion":"unknown"}}`

type antigravityWaitingSummary struct {
	Waiting []struct {
		Index *uint32 `json:"stepIndex"`
		Step  struct {
			Status   json.RawMessage
			Metadata struct {
				Execution string `json:"executionId"`
				Source    struct {
					Cascade    string `json:"cascadeId"`
					Trajectory string `json:"trajectoryId"`
					Index      uint32 `json:"stepIndex"`
				} `json:"sourceTrajectoryStepInfo"`
			}
			Interaction struct {
				Permission *struct {
					Resource struct{ Action, Target string }
				}
				Question *struct {
					Questions []struct{ Question string }
				} `json:"askQuestion"`
				QuestionSnake *struct {
					Questions []struct{ Question string }
				} `json:"ask_question"`
			} `json:"requestedInteraction"`
		}
	} `json:"waitingSteps"`
}

// The endpoint includes historical cascades even when nothing is waiting.
// Decode one summary at a time so the JSON buffer does not grow with history.
func decodeAntigravityWaits(r io.Reader) ([]AntigravityWait, error) {
	dec := json.NewDecoder(io.LimitReader(r, 8<<20))
	open, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if open != json.Delim('{') {
		return nil, errors.New("invalid Antigravity request summary")
	}
	pending := map[string][]AntigravityWait{}
	for dec.More() {
		field, err := dec.Token()
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(field.(string), "trajectorySummaries") {
			if err := dec.Decode(&ignoreAntigravityField{}); err != nil {
				return nil, err
			}
			continue
		}
		open, err := dec.Token()
		if err != nil {
			return nil, err
		}
		if open == nil {
			clear(pending)
			continue
		}
		if open != json.Delim('{') {
			return nil, errors.New("invalid Antigravity cascade summaries")
		}
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return nil, err
			}
			var summary antigravityWaitingSummary
			if err := dec.Decode(&summary); err != nil {
				return nil, err
			}
			cascade := key.(string)
			if waits := pendingAntigravityWaits(cascade, summary); len(waits) > 0 {
				pending[cascade] = waits
			} else {
				delete(pending, cascade)
			}
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	var waits []AntigravityWait
	for _, list := range pending {
		waits = append(waits, list...)
	}
	return waits, nil
}

type ignoreAntigravityField struct{}

func (*ignoreAntigravityField) UnmarshalJSON([]byte) error { return nil }

func pendingAntigravityWaits(cascade string, summary antigravityWaitingSummary) []AntigravityWait {
	var waits []AntigravityWait
	for _, entry := range summary.Waiting {
		step := entry.Step
		source := step.Metadata.Source
		if string(step.Status) != `"CORTEX_STEP_STATUS_WAITING"` && string(step.Status) != "9" {
			continue
		}
		if cascade == "" || source.Trajectory == "" || source.Cascade != "" && source.Cascade != cascade ||
			entry.Index != nil && *entry.Index != source.Index {
			continue
		}
		wait := AntigravityWait{"", cascade, source.Trajectory, step.Metadata.Execution, source.Index, false}
		if p := step.Interaction.Permission; p != nil {
			if strings.TrimSpace(p.Resource.Action) != "" && strings.TrimSpace(p.Resource.Target) != "" {
				wait.Permission = true
				waits = append(waits, wait)
			}
			continue
		}
		q := step.Interaction.Question
		if q == nil {
			q = step.Interaction.QuestionSnake
		}
		if q != nil {
			for _, item := range q.Questions {
				if strings.TrimSpace(item.Question) != "" {
					waits = append(waits, wait)
					break
				}
			}
		}
	}
	return waits
}

var antigravityDataDirs = [...]string{"antigravity", "antigravity-cli", "antigravity-ide"}

func registeredAntigravityDaemons(homes []string) map[daemonAddress]antigravityDaemon {
	found := make(map[daemonAddress]antigravityDaemon)
	for _, home := range homes {
		if !filepath.IsAbs(home) {
			continue
		}
		for _, folder := range antigravityDataDirs {
			dir := filepath.Join(home, folder, "daemon")
			entries, _ := os.ReadDir(dir)
			for _, entry := range entries {
				if filepath.Ext(entry.Name()) != ".json" || !entry.Type().IsRegular() {
					continue
				}
				f, err := os.Open(filepath.Join(dir, entry.Name()))
				if err != nil {
					continue
				}
				var d antigravityDaemon
				err = json.NewDecoder(io.LimitReader(f, 4096)).Decode(&d)
				_ = f.Close()
				if err != nil || d.PID == 0 || d.HTTPSPort == 0 || d.Token == "" || !isAntigravityProcess(d.PID) {
					continue
				}
				key := daemonAddress{d.PID, d.HTTPSPort}
				found[key] = d
			}
		}
	}
	return found
}

func isAntigravityExecutable(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	if name == "agy" || name == "agy.exe" {
		return true
	}
	path = strings.ToLower(filepath.ToSlash(path))
	return (strings.Contains(name, "language_server") || strings.Contains(name, "language-server")) &&
		strings.Contains(path, "antigravity")
}

func newAntigravityTransport(d antigravityDaemon, owns func(uint32, *net.TCPAddr, *net.TCPAddr) bool) *http.Transport {
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(int(d.HTTPSPort)))
	return &http.Transport{
		// Antigravity uses a self-signed cert. Verify the actual TCP peer by
		// its registered OS process before TLS and before sending the token.
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		IdleConnTimeout: 15 * time.Second,
		DialContext: func(ctx context.Context, _, target string) (net.Conn, error) {
			if target != address {
				return nil, errors.New("unexpected Antigravity address")
			}
			var dialer net.Dialer
			conn, err := dialer.DialContext(ctx, "tcp4", address)
			if err != nil {
				return nil, err
			}
			local, remote := conn.LocalAddr().(*net.TCPAddr), conn.RemoteAddr().(*net.TCPAddr)
			// The accepted server socket may not be visible until accept returns.
			for i := 0; i < 4; i++ {
				if owns(d.PID, local, remote) {
					return conn, nil
				}
				if i != 3 {
					select {
					case <-ctx.Done():
						_ = conn.Close()
						return nil, ctx.Err()
					case <-time.After(15 * time.Millisecond):
					}
				}
			}
			_ = conn.Close()
			return nil, errors.New("Antigravity TCP peer does not match its registered process")
		},
	}
}

func readAntigravityWaits(ctx context.Context, client *http.Client, d antigravityDaemon) ([]AntigravityWait, error) {
	res, err := antigravityQuery(ctx, client, d, antigravityReadPath, "{}")
	if err != nil {
		return nil, err
	}
	defer res.Close()
	return decodeAntigravityWaits(res)
}

// A daemon directory can be shared, and the active account can change in a
// running client. Attribute a request only to the account the client reports.
func antigravityAccount(ctx context.Context, client *http.Client, d antigravityDaemon) (string, error) {
	res, err := antigravityQuery(ctx, client, d, antigravityIdentityPath, antigravityMetadata)
	if err != nil {
		return "", err
	}
	defer res.Close()
	var output struct {
		UserStatus struct{ Email string }
	}
	err = json.NewDecoder(io.LimitReader(res, 4<<20)).Decode(&output)
	return strings.ToLower(strings.TrimSpace(output.UserStatus.Email)), err
}

func antigravityQuery(ctx context.Context, client *http.Client, d antigravityDaemon, path, payload string) (io.ReadCloser, error) {
	url := "https://127.0.0.1:" + strconv.Itoa(int(d.HTTPSPort)) + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("X-Codeium-Csrf-Token", d.Token)
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		return nil, errors.New("Antigravity local status unavailable")
	}
	return res.Body, nil
}

func antigravityWaitHomes(profiles []AntigravityProfile) []string {
	homes := make([]string, 0, len(profiles)+1)
	home := os.Getenv("GEMINI_CLI_HOME")
	if !filepath.IsAbs(home) {
		home = ""
		if userHome, err := os.UserHomeDir(); err == nil {
			home = filepath.Join(userHome, ".gemini")
		}
	}
	if home != "" {
		homes = append(homes, filepath.Clean(home))
	}
	for _, p := range profiles {
		if filepath.IsAbs(p.GeminiHome) {
			home := filepath.Clean(p.GeminiHome)
			if !slices.Contains(homes, home) {
				homes = append(homes, home)
			}
		}
	}
	return homes
}

func WatchAntigravityWaits(ctx context.Context, profiles []AntigravityProfile) <-chan []AntigravityWait {
	homes := antigravityWaitHomes(profiles)
	if len(homes) == 0 {
		return nil
	}
	out := make(chan []AntigravityWait, 1)
	go func() {
		defer close(out)
		tick := time.NewTicker(3 * time.Second)
		defer tick.Stop()
		type source struct {
			record     antigravityDaemon
			transport  *http.Transport
			client     *http.Client
			waits      []AntigravityWait
			account    string
			identityAt time.Time
		}
		sources := make(map[daemonAddress]source)
		defer func() {
			for _, src := range sources {
				src.transport.CloseIdleConnections()
			}
		}()
		var before map[AntigravityWait]struct{}
		for {
			known := registeredAntigravityDaemons(homes)
			for key, src := range sources {
				if got, ok := known[key]; !ok || got != src.record {
					src.transport.CloseIdleConnections()
					delete(sources, key)
				}
			}
			next := make(map[AntigravityWait]struct{})
			for key, record := range known {
				src, ok := sources[key]
				if !ok {
					tr := newAntigravityTransport(record, antigravityOwnsPeer)
					src = source{record: record, transport: tr, client: &http.Client{
						Transport: tr, Timeout: 1500 * time.Millisecond,
						CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
					}}
				}
				if waits, err := readAntigravityWaits(ctx, src.client, record); err == nil {
					src.waits = waits
					if len(waits) == 0 {
						src.account, src.identityAt = "", time.Time{}
					} else if time.Since(src.identityAt) >= 15*time.Second {
						if account, err := antigravityAccount(ctx, src.client, record); err == nil {
							src.account, src.identityAt = account, time.Now()
						}
					}
				}
				sources[key] = src
				for _, wait := range src.waits {
					wait.Account = src.account
					next[wait] = struct{}{}
				}
			}
			if !maps.Equal(before, next) {
				waits := make([]AntigravityWait, 0, len(next))
				for item := range next {
					waits = append(waits, item)
				}
				sort.Slice(waits, func(i, j int) bool {
					a, b := waits[i], waits[j]
					if a.Account != b.Account {
						return a.Account < b.Account
					}
					if a.Cascade != b.Cascade {
						return a.Cascade < b.Cascade
					}
					return a.Step < b.Step
				})
				select {
				case out <- waits:
					before = next
				case <-ctx.Done():
					return
				}
			}
			select {
			case <-tick.C:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
