package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/leihog/discord-bot/internal/database"
	luaengine "github.com/leihog/discord-bot/internal/lua"
)

// devSession implements luaengine.MessageSender; it sends bot messages into the TUI.
type devSession struct {
	mu sync.Mutex
	p  *tea.Program
}

func (d *devSession) ChannelMessageSend(channelID, content string, _ ...discordgo.RequestOption) (*discordgo.Message, error) {
	d.p.Send(botMsgEvent{channelID: channelID, content: content})
	return nil, nil
}

// teaLogWriter redirects log output into the TUI viewport.
type teaLogWriter struct{ p *tea.Program }

func (w *teaLogWriter) Write(b []byte) (int, error) {
	w.p.Send(logLineEvent{line: strings.TrimRight(string(b), "\n")})
	return len(b), nil
}

// tea.Msg types
type botMsgEvent struct{ channelID, content string }
type logLineEvent struct{ line string }
type execDoneEvent struct {
	output string
	err    error
}
type scriptNamesEvent struct{ names []string }

// shellState tracks the simulated user/channel context.
type shellState struct {
	author    string
	authorID  string
	channelID string
	dmMode    bool
}

type model struct {
	viewport   viewport.Model
	input      textinput.Model
	state      shellState
	lines      []string
	engine     *luaengine.Engine
	scriptsDir string
	cancel     context.CancelFunc
	ready      bool
	width      int
	height     int
}

func newModel(engine *luaengine.Engine, scriptsDir string, cancel context.CancelFunc) model {
	ti := textinput.New()
	ti.Focus()

	m := model{
		input:      ti,
		engine:     engine,
		scriptsDir: scriptsDir,
		cancel:     cancel,
		state: shellState{
			author:    "dev",
			authorID:  "dev-user",
			channelID: "dev-channel",
		},
	}
	m.updatePrompt()
	return m
}

func (m *model) Init() tea.Cmd {
	return textinput.Blink
}

func (m *model) View() string {
	if !m.ready {
		return "Starting dev shell..."
	}
	return fmt.Sprintf("%s\n%s", m.viewport.View(), m.input.View())
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-1)
			m.viewport.SetContent(strings.Join(m.lines, "\n"))
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 1
		}
		m.input.Width = msg.Width - len(m.input.Prompt)

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.cancel()
			return m, tea.Quit
		case tea.KeyEnter:
			line := m.input.Value()
			m.input.SetValue("")
			if line != "" {
				m.addLine(fmt.Sprintf("[#%s] %s> %s", m.state.channelID, m.state.author, line))
				if strings.HasPrefix(line, "/") {
					if cmd := m.handleMeta(line); cmd != nil {
						cmds = append(cmds, cmd)
					}
				} else {
					m.engine.ProcessMessage(buildMessage(line, &m.state))
				}
			}
		case tea.KeyShiftUp:
			m.viewport.ScrollUp(3)
		case tea.KeyShiftDown:
			m.viewport.ScrollDown(3)
		default:
			var inputCmd tea.Cmd
			m.input, inputCmd = m.input.Update(msg)
			cmds = append(cmds, inputCmd)
		}

	case botMsgEvent:
		m.addLine(fmt.Sprintf("[Bot → #%s]: %s", msg.channelID, msg.content))

	case logLineEvent:
		m.addLine(msg.line)

	case execDoneEvent:
		if msg.err != nil {
			m.addLine(fmt.Sprintf("Error: %v", msg.err))
		} else if msg.output != "" {
			for l := range strings.SplitSeq(msg.output, "\n") {
				if l != "" {
					m.addLine("→ " + l)
				}
			}
		}

	case scriptNamesEvent:
		if len(msg.names) == 0 {
			m.addLine("No scripts loaded.")
		} else {
			for _, name := range msg.names {
				m.addLine("  " + name)
			}
		}

	default:
		var vpCmd, inputCmd tea.Cmd
		m.viewport, vpCmd = m.viewport.Update(msg)
		m.input, inputCmd = m.input.Update(msg)
		cmds = append(cmds, vpCmd, inputCmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *model) addLine(line string) {
	m.lines = append(m.lines, line)
	m.viewport.SetContent(strings.Join(m.lines, "\n"))
	m.viewport.GotoBottom()
}

func (m *model) updatePrompt() {
	prefix := ""
	if m.state.dmMode {
		prefix = "(DM) "
	}
	m.input.Prompt = fmt.Sprintf("%s[#%s] %s> ", prefix, m.state.channelID, m.state.author)
	if m.width > 0 {
		m.input.Width = m.width - len(m.input.Prompt)
	}
}

func (m *model) handleMeta(line string) tea.Cmd {
	parts := strings.Fields(line)
	cmd := parts[0]

	switch cmd {
	case "/help":
		m.addLine(`Shell commands:
  /channel <id>       Set active channel ID
  /user <name> [id]   Set author name and optional ID
  /dm                 Toggle DM mode (on_direct_message vs on_channel_message)
  /scripts            List loaded scripts
  /reload <name>      Reload a script by name (e.g. jokes.lua)
  /commands           List registered commands
  /hooks              List registered hooks
  /lua <code>         Execute Lua code and print result
  /quit, /exit        Exit the shell`)

	case "/channel":
		if len(parts) < 2 {
			m.addLine("Usage: /channel <id>")
			return nil
		}
		m.state.channelID = parts[1]
		m.updatePrompt()
		m.addLine(fmt.Sprintf("Channel set to #%s", m.state.channelID))

	case "/user":
		if len(parts) < 2 {
			m.addLine("Usage: /user <name> [id]")
			return nil
		}
		m.state.author = parts[1]
		if len(parts) >= 3 {
			m.state.authorID = parts[2]
		} else {
			m.state.authorID = parts[1]
		}
		m.updatePrompt()
		m.addLine(fmt.Sprintf("User set to %s (id: %s)", m.state.author, m.state.authorID))

	case "/dm":
		m.state.dmMode = !m.state.dmMode
		m.updatePrompt()
		if m.state.dmMode {
			m.addLine("DM mode: ON  (messages trigger on_direct_message)")
		} else {
			m.addLine("DM mode: OFF (messages trigger on_channel_message)")
		}

	case "/hooks":
		hooks := m.engine.GetHookNames()
		if len(hooks) == 0 {
			m.addLine("No hooks registered.")
		} else {
			for hookName, scripts := range hooks {
				m.addLine(fmt.Sprintf("  %s: %s", hookName, strings.Join(scripts, ", ")))
			}
		}

	case "/reload":
		if len(parts) < 2 {
			m.addLine("Usage: /reload <name>")
			return nil
		}
		scriptPath := filepath.Join(m.scriptsDir, parts[1])
		m.engine.EnqueueScriptEvent(scriptPath, "reload")
		m.addLine(fmt.Sprintf("Reloading %s...", parts[1]))

	case "/scripts":
		engine := m.engine
		return func() tea.Msg {
			return scriptNamesEvent{names: engine.GetScriptNames()}
		}

	case "/commands":
		engine := m.engine
		return func() tea.Msg {
			out, err := engine.Exec(`
local cmds = get_commands()
local found = false
for name, cmd in pairs(cmds) do
    print(name .. " - " .. cmd.description .. " [" .. cmd.script .. "]")
    found = true
end
if not found then print("No commands registered.") end
`)
			return execDoneEvent{output: out, err: err}
		}

	case "/lua":
		if len(parts) < 2 {
			m.addLine("Usage: /lua <code>")
			return nil
		}
		code := strings.TrimPrefix(line, "/lua ")
		engine := m.engine
		return func() tea.Msg {
			out, err := engine.Exec(code)
			return execDoneEvent{output: out, err: err}
		}

	case "/quit", "/exit":
		m.cancel()
		return tea.Quit

	default:
		m.addLine(fmt.Sprintf("Unknown command: %s (type /help for help)", cmd))
	}

	return nil
}

func buildMessage(content string, state *shellState) *discordgo.MessageCreate {
	guildID := "dev-guild"
	if state.dmMode {
		guildID = ""
	}
	return &discordgo.MessageCreate{
		Message: &discordgo.Message{
			Content:   content,
			ChannelID: state.channelID,
			GuildID:   guildID,
			Author: &discordgo.User{
				Username: state.author,
				ID:       state.authorID,
			},
		},
	}
}

func main() {
	scriptsDir := flag.String("scripts-dir", "scripts", "path to scripts directory")
	dbPath := flag.String("db", ":memory:", "SQLite database path")
	flag.Parse()

	db, err := database.New(*dbPath)
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}
	defer db.Close()

	if err := db.Initialize(); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	sess := &devSession{}
	engine := luaengine.New(db, sess)
	engine.Initialize()
	engine.LoadScripts(*scriptsDir)

	ctx, cancel := context.WithCancel(context.Background())
	engine.Start(ctx)
	luaengine.NewWatcher(engine, *scriptsDir).Start(ctx)

	m := newModel(engine, *scriptsDir, cancel)
	p := tea.NewProgram(&m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	sess.p = p
	log.SetOutput(&teaLogWriter{p: p})

	if _, err := p.Run(); err != nil {
		log.Fatal("TUI error:", err)
	}

	engine.Close()
}
