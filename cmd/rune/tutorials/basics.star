# basics.star — the first-run tutorial.
#
# Walks the user through the three things they need to know to be
# productive in Rune: where to put work in the empty workspace,
# how to open a workspace (`:workspaceopen`), and how to start editing
# files inside that workspace (`:edit`), plus the layout model
# (the meta/alt/shift system) and the windows and tabs that hold
# that content.
#
# The DSL is interpreted by ide/idetutorial/starlarktutorial. The
# entry function runs on its own Starlark goroutine; each blocking
# builtin (floating_window, wait_command, confirm) returns a real
# value so authors can branch, loop, and compose helpers with
# regular Starlark control flow.

ck = command_key()
mode = editor_mode()

# Direction keys differ by editor mode: modal points with the home
# row, modeless points with the arrow keys. The layout copy adapts to
# whichever the user is running.
if mode == "modal":
    dir_phrase = "the home row, `h` `j` `k` `l`"
    focus_example = "`<meta-h>`"
else:
    dir_phrase = "the arrow keys"
    focus_example = "`<meta-left>`"

def keyhint(cmd, *args):
    k = key_for(cmd, *args)
    return (" Default key: `" + k + "`.") if k else ""

def keypress(cmd, *args):
    # The user's real bound key for cmd, phrased as a keypress
    # instruction. Falls back to the command-prompt wording when the
    # command is unbound, so copy adapts to mode and custom rebinds.
    k = key_for(cmd, *args)
    if k:
        return "press `" + k + "`"
    return "open the command prompt (`" + ck + "`) and run `" + cmd + "`"

# In modal mode the console's input line captures keys while you are in
# insert mode, so `:` types a literal colon instead of opening the
# command prompt. The user must press <esc> to enter modal mode first.
# These snippets are spliced into the agent steps that open the command
# prompt while the companion console is focused.
if mode == "modal":
    shell_esc_step = "1. Press `<esc>` to enter modal mode.\n"
    shell_prompt_step_num = "2"
    shell_run_step_num = "3"
else:
    shell_esc_step = ""
    shell_prompt_step_num = "1"
    shell_run_step_num = "2"

def dismiss_for(cmd, *args):
    # Keys that dismiss a teaching window. Always include the command
    # prompt key. When the command has a bound key, include it too so
    # the single keypress the copy asks for dismisses the window AND
    # falls through to the IDE, dispatching the command that the
    # following wait_command observes.
    k = key_for(cmd, *args)
    return [ck, k] if k else [ck]

welcome_md = """\
This is the **home workspace**: a scratch workspace rooted at `~/` that
Rune shows when no project workspace is open at the current slot.
Use it for files outside any project, quick terminals,
or to keep notes between sessions.

## Switching workspaces

Rune has **nine workspace slots**. Press `<meta-1>` through
`<meta-9>` to jump between them. Every empty slot shows this same home
workspace; a slot only gets a project attached when you open one
inside it. So slot 1 may be the project you're working on while
slots 2-9 are still the home workspace, ready for whatever you
need.

## Commands and key bindings

IDE-wide operations are exposed as **commands** that you invoke
from the command prompt. Keys like `<meta-1>` and `<meta-enter>`
are bound to those commands through your user configuration under
`command.key_bindings`, so every binding shown here is rebindable.

## Opening a workspace

1. Press `""" + ck + """` to open the command prompt.
2. Type `workspaceopen` and use the auto-completer (Tab / arrow keys)
   to pick a workspace.
3. Press Enter to open it.

Press `<enter>` or `<space>` to continue.
"""

edit_md = """\
You're in a real workspace now. To open a file:

1. Press `""" + ck + """` to open the command prompt.
2. Type `edit` followed by a partial filename.
3. Use the auto-completer to pick the file you want.
4. Press Enter.

Press `<enter>` or `<space>` to continue.
"""

layout_md = """\
Rune is a full tiling window manager: you split the screen into
**windows**, fill each one with **tabs** (files, terminals, task
output), and group whole projects into **workspaces**. The editor is
just one kind of content among many. Every layout action is a command
you can type at the prompt; the default keys are just shortcuts, and
they follow one small, standardized system.

## The three layers

- **`<meta>` is the window and workspace layer.** Anything that
  focuses, moves, or splits a window, or switches a workspace, is a
  `<meta>` chord. (On macOS `<meta>` is Command; on Linux and Windows
  it is the Super or Windows key.)
- **`<alt>` is the tab layer.** Switching or reordering the tabs
  inside a window lives on `<alt>`.
- **`<shift>` means "move" instead of "go to".** `<meta>` plus a
  direction *focuses* a window; add `<shift>` and the same direction
  *moves* it. The rule holds for tabs and workspaces too.

Press `<enter>` or `<space>` to continue.
"""

split_window_md = """\
A **window** is a tile on screen, and right now this workspace has just
one.

Split the focused window into two: """ + keypress("windownew") + """.
"""

terminal_md = """\
The new pane is empty, and windows hold any kind of content, not just
files. Fill this one with a terminal.

Open a terminal here: """ + keypress("terminalneworsplit") + """.
"""

modal_surfaces_md = """\
You picked **modal** editor mode, and in modal mode every input surface
is modal, not just the editor. This includes the terminal, Rune's
console, and the file explorer.

The cursor shape tells you which mode a surface is in: a block cursor
means NORMAL mode, a bar cursor means INSERT mode.

So when a terminal or console is focused and you want to open the command
prompt (`""" + ck + """`), first switch back to NORMAL mode with
`<esc>`.
"""

split_direction_md = """\
Splits stack in one direction until you flip it. Flip it now so the
next split lands the other way.

Flip the split direction: """ + keypress("windowdefaultsplit", "h") + """.
"""

terminal_split_md = """\
The same key opens a terminal in a new split when the window already
has content. Do it again and watch the split land along the direction
you just set.

Open another terminal split: """ + keypress("terminalneworsplit") + """.
"""

focus_window_md = """\
The screen is split into several windows now. `<meta>` plus a direction
moves focus across the splits, so you can hop between them without the
mouse.

- `windowfocus left` focuses the window to the left.""" + keyhint("windowfocus", "left") + """
- `windowfocus right` focuses the window to the right.""" + keyhint("windowfocus", "right") + """

First focus the window on the left (""" + keypress("windowfocus", "left") + """),
then the window on the right (""" + keypress("windowfocus", "right") + """).
"""

move_window_md = """\
`<shift>` turns "go to" into "move". So `<meta>` plus a direction
focuses a window, and adding `<shift>` *moves* the focused window's
content that way instead, swapping it with the neighbor.

- `windowmove left` moves the focused window to the left.""" + keyhint("windowmove", "left") + """
- `windowmove right` moves it to the right.""" + keyhint("windowmove", "right") + """

Move the focused window to the left (""" + keypress("windowmove", "left") + """),
then back to the right (""" + keypress("windowmove", "right") + """).
"""

fullscreen_window_md = """\
When you want to focus on one window, `windowtogglemaximize` grows it
to fill the whole editor area. Run it again, or focus another window,
to restore the layout.""" + keyhint("windowtogglemaximize") + """

Toggle fullscreen now: """ + keypress("windowtogglemaximize") + """.
"""

close_window_md = """\
`windowclose` closes the focused split and moves focus to the next
window. It will not close your last window: a workspace always keeps at
least one.""" + keyhint("windowclose") + """

To close the split you just made, """ + keypress("windowclose") + """.
"""

tabs_md = """\
A window shows one **tab** at a time: a file, a terminal, task output.
You already have one open. Let's add another from the file explorer.

Open the file explorer: """ + keypress("fexplorer") + """.
"""

tab_open_file_md = """\
The explorer is the workspace tree, and it is a live buffer: edit a
name to rename, add a line to create, delete a line to remove, then
save the buffer via `write` to apply. For now, just open a file.

Move to a file with """ + dir_phrase + """ and press `<enter>`. It opens
as a new tab in the window you came from.
"""

tab_close_explorer_md = """\
`fexplorer` is a toggle: the same key that opened the explorer closes
it again. With your file open, you no longer need the tree taking up
space.

Toggle the explorer closed: """ + keypress("fexplorer") + """.
"""

tab_switch_md = """\
That window now holds two tabs. `tabnext` / `tabprevious` cycle through
them and wrap around; tabs are the `<alt>` layer.

Switch to the next tab (""" + keypress("tabnext") + """), then back to
the previous one (""" + keypress("tabprevious") + """).
"""

close_tab_md = """\
`tabclose` closes the focused tab; the next tab in the list takes its
place.""" + keyhint("tabclose") + """

To close the current tab, """ + keypress("tabclose") + """.
"""

terminals_md = """\
Rune runs terminals as content, so they live in windows and tabs just
like files. Rune can also run one-shot programs on an ephemeral terminal,
interactively.

## Running a program

- `! <cmd>` runs a program in a floating window that shows its output,
  for example `! git log`.
- `!!` runs a program but hides its output. Use this when you only cares
  whether it worked or not.

Let's run `git log`. At the command prompt (`""" + ck + """`), type
`! git log` and press Enter. Rune opens a floating window streaming its
output.
"""

terminals_close_md = """\
The `git log` output is sitting in a floating window. It is a window,
not a tab, so close it with `windowclose`.

Close it now: """ + keypress("windowclose") + """.
"""

console_intro_md = """\
Next you'll meet the **Rune console**. It is not the command prompt
you have been using, so first, the difference.

The **command prompt** (`""" + ck + """`) is the one-line prompt you
open, type one command into, and watch close again once it runs. Good
for one-off actions like opening a file, splitting a window, or
jumping to a definition.

The **console** is a separate, durable tab with its own REPL. It is
wired with commands that benefit from a persistent output window. For example
checking the status of an extension, or installing a new package.

The console is used to **set up** Rune, the prompt is used to **drive** it.

Press `<enter>` or `<space>` to continue.
"""

agent_install_md = """\
The **Rune Agent** is Rune's builtin AI coding assistant. It ships as a
package you install on demand, so the first step is to install it.

First, open Rune's console:

1. Press `""" + ck + """` to open the command prompt.
2. Type `console` and press Enter.

Press `<enter>` or `<space>` to continue.
"""

agent_pkg_install_md = """\
You're in Rune's console now. Install the agent package:

1. Type `pkg install rune-agent`.
2. Press Enter and wait for the install to finish.

Press `<enter>` or `<space>` to continue.
"""

agent_open_md = """\
Start a conversation with the agent using the `agent` command.

""" + shell_esc_step + shell_prompt_step_num + """. Press `""" + ck + """` to open the command prompt.
""" + shell_run_step_num + """. Run `agent`.

`agent` takes two optional arguments: a conversation name and a model.
Run `agent <name>` to name the conversation, or `agent <name> <model>`
to also pick the model. With no arguments, the agent starts a new
conversation using your default provider.

Press `<enter>` or `<space>` to continue.
"""

cheatsheet_md = """\
There's a `cheatsheet` command that condenses all of this tutorial's
learnings and more into a single cheat sheet you can pull up any time.

Open it now: """ + keypress("cheatsheet") + """.
"""

help_md = """\
You're almost done. A few tips worth remembering:

- If you find yourself wondering what commands you typed on a previous session, press
  `<meta-r>` to open the command prompt in history mode and search through your command history.
- There's a `help` command that opens the documentation on a separate workspace
  and fires a help agent that you can ask questions.

Try it now: """ + keypress("help") + """. Happy hacking!
"""

def teach_edit():
    floating_window(title = "Open a file", text = edit_md, dismiss_keys = [ck])
    edit = wait_command(
        title    = "Open a file",
        command  = "edit",
        on_error = ("`<cmd>edit` needs a `<file>` argument. Use the " +
                    "auto-completer (Tab / arrow keys) to pick a " +
                    "file, or type a path inside the workspace; " +
                    "relative paths resolve against the workspace root."),
    )
    notify(level = success, message = "You opened " + edit.args[0])


def teach_layout():
    floating_window(title = "Layout management", text = layout_md,
                    dismiss_keys = [ck])


def teach_split_window():
    floating_window(title = "Split a window", text = split_window_md,
                    dismiss_keys = dismiss_for("windownew"))
    wait_command(
        title    = "Split a window",
        command  = "windownew",
        on_error = ("Split the active window into two. Add an optional " +
                    "direction (`<cmd>windownew right` / `left` / `up` / " +
                    "`down`) to choose where the new pane lands."),
    )
    notify(level = success, message = "You split the window.")


def teach_terminal():
    floating_window(title = "Open a terminal", text = terminal_md,
                    dismiss_keys = dismiss_for("terminalneworsplit"))
    wait_command(
        title    = "Open a terminal",
        command  = "terminalneworsplit",
        on_error = ("Open a terminal in the focused window. When the window " +
                    "is empty the terminal fills it in place."),
    )
    notify(level = success, message = "You opened a terminal.")


def teach_modal_surfaces():
    if mode != "modal":
        return
    floating_window(title = "Modal everywhere", text = modal_surfaces_md,
                    dismiss_keys = [ck])


def teach_split_direction():
    floating_window(title = "Flip the split direction", text = split_direction_md,
                    dismiss_keys = dismiss_for("windowdefaultsplit", "h"))
    wait_command(
        title    = "Flip the split direction",
        command  = "windowdefaultsplit",
        on_error = ("Flip the default split direction. Pass an orientation " +
                    "to set it outright (`<cmd>windowdefaultsplit horizontal` " +
                    "/ `vertical`)."),
    )
    notify(level = success, message = "You changed the split direction.")


def teach_terminal_split():
    floating_window(title = "Open a terminal split", text = terminal_split_md,
                    dismiss_keys = dismiss_for("terminalneworsplit"))
    wait_command(
        title    = "Open a terminal split",
        command  = "terminalneworsplit",
        on_error = ("Split the focused window and open a terminal in the new " +
                    "pane. Because the window already has content, it opens " +
                    "a split instead of filling it in place."),
    )
    notify(level = success, message = "You opened a terminal split.")


def teach_focus_window():
    floating_window(title = "Move between windows", text = focus_window_md,
                    dismiss_keys = dismiss_for("windowfocus", "left"))
    wait_command(
        title    = "Move between windows",
        command  = "windowfocus",
        on_error = "Focus the window to the left.",
    )
    wait_command(
        title    = "Move between windows",
        command  = "windowfocus",
        on_error = "Now focus the window to the right.",
    )
    notify(level = success, message = "You moved between windows.")


def teach_move_window():
    floating_window(title = "Move a window", text = move_window_md,
                    alignment = "top",
                    dismiss_keys = dismiss_for("windowmove", "left"))
    wait_command(
        title    = "Move a window",
        command  = "windowmove",
        on_error = "Move the focused window to the left.",
    )
    wait_command(
        title    = "Move a window",
        command  = "windowmove",
        on_error = "Now move it back to the right.",
    )
    notify(level = success, message = "You moved a window.")


def teach_fullscreen_window():
    floating_window(title = "Fullscreen a window", text = fullscreen_window_md,
                    dismiss_keys = dismiss_for("windowtogglemaximize"))
    wait_command(
        title    = "Fullscreen a window",
        command  = "windowtogglemaximize",
        on_error = "Toggle the focused window to fullscreen and back.",
    )
    notify(level = success, message = "You toggled fullscreen.")


def teach_close_window():
    floating_window(title = "Close a window", text = close_window_md,
                    dismiss_keys = dismiss_for("windowclose"))
    wait_command(
        title    = "Close a window",
        command  = "windowclose",
        on_error = ("Close the focused split. It will not close your last " +
                    "window."),
    )
    notify(level = success, message = "You closed the window.")


def teach_tabs():
    floating_window(title = "Open another tab", text = tabs_md,
                    dismiss_keys = dismiss_for("fexplorer"))
    wait_command(
        title    = "Open another tab",
        command  = "fexplorer",
        on_error = "Open the file explorer with `<cmd>fexplorer`.",
    )
    notify(level = success, message = "File explorer open.")

    wait_event(
        event    = "open",
        title    = "Open a file",
        text     = tab_open_file_md,
        on_error = "Move to a file in the explorer and press `<enter>` to open it.",
    )
    notify(level = success, message = "You opened a file in a new tab.")

    floating_window(title = "Close the explorer", text = tab_close_explorer_md,
                    dismiss_keys = dismiss_for("fexplorer"))
    wait_command(
        title    = "Close the explorer",
        command  = "fexplorer",
        on_error = "Toggle the file explorer closed with `<cmd>fexplorer`.",
    )
    notify(level = success, message = "File explorer closed.")


def teach_switch_tabs():
    floating_window(title = "Switch tabs", text = tab_switch_md,
                    dismiss_keys = dismiss_for("tabnext"))
    wait_command(
        title    = "Switch tabs",
        command  = "tabnext",
        on_error = ("Move to the next tab in this window. If the window " +
                    "only has one tab, open a second file from the " +
                    "explorer first."),
    )
    wait_command(
        title    = "Switch tabs",
        command  = "tabprevious",
        on_error = "Now move back to the previous tab.",
    )
    notify(level = success, message = "You switched tabs.")


def teach_close_tab():
    floating_window(title = "Close a tab", text = close_tab_md,
                    dismiss_keys = dismiss_for("tabclose"))
    wait_command(
        title    = "Close a tab",
        command  = "tabclose",
        on_error = "Close the focused tab.",
    )
    notify(level = success, message = "You closed the tab.")


def teach_terminals():
    floating_window(title = "Run a program", text = terminals_md,
                    dismiss_keys = [ck])
    wait_command(
        title    = "Run a program",
        command  = "!",
        on_error = "At the command prompt, run `<cmd>! git log`.",
    )
    notify(level = success, message = "Program running in a window.")

    floating_window(title = "Close the output window", text = terminals_close_md,
                    dismiss_keys = dismiss_for("windowclose"))
    wait_command(
        title    = "Close the output window",
        command  = "windowclose",
        on_error = "Close the `git log` output window with `<cmd>windowclose`.",
    )
    notify(level = success, message = "Output window closed.")


def teach_provider(provider, label, action_tokens, run_md, success_msg):
    title = "Connect " + label
    floating_window(title = title, text = run_md)
    wait_shell(
        title    = title,
        args     = ["models", "providers", provider] + action_tokens,
        on_error = ("In Rune's console, run `models providers " +
                    provider + " " + " ".join(action_tokens) + "`."),
    )
    notify(level = success, message = success_msg)


def teach_agent():
    floating_window(title = "The Rune console", text = console_intro_md)

    floating_window(title = "Set up the Rune Agent", text = agent_install_md,
                    dismiss_keys = [ck])
    wait_command(
        title    = "Set up the Rune Agent",
        command  = "console",
        on_error = ("Open Rune's console: run the `<cmd>console` " +
                    "command."),
    )

    floating_window(title = "Install the agent package", text = agent_pkg_install_md)
    wait_shell(
        title    = "Install the agent package",
        args     = ["pkg", "install", "rune-agent"],
        on_error = "In Rune's console, run `pkg install rune-agent`.",
    )
    notify(level = success, message = "Rune Agent installed.")

    pick = choice(
        message = ("Which provider do you want to connect?\n\n" +
                   "- **OpenAI** — GPT family, billed by API key.\n" +
                   "- **Anthropic** — Claude family, billed by API key.\n" +
                   "- **Gemini** — Gemini family, billed by API key.\n" +
                   "- **Codex** — GPT-5 Codex family. Sign in with " +
                   "ChatGPT (OAuth) and use your ChatGPT subscription, " +
                   "no API key.\n" +
                   "- **Claude** — Claude family through your Claude " +
                   "Pro/Max subscription. Sign in with Claude (OAuth), " +
                   "no API key."),
        options = ["OpenAI", "Anthropic", "Gemini", "Codex", "Claude"],
    )
    if not pick.selected:
        notify(level = info,
               message = ("Configure a provider any time with the " +
                          "`models providers` console command."))
        return

    if pick.value == "OpenAI":
        run_md = """\
Add your OpenAI credentials. The key is stored securely and never
written to your config file.

1. In Rune's console, run `models providers openai add default`.
2. Paste your API key at the redacted prompt.

Press `<enter>` or `<space>` to continue.
"""
        teach_provider("openai", "OpenAI", ["add", "default"], run_md,
                       "OpenAI connected.")
    elif pick.value == "Anthropic":
        run_md = """\
Add your Anthropic credentials. The key is stored securely and never
written to your config file.

1. In Rune's console, run `models providers anthropic add default`.
2. Paste your API key at the redacted prompt.

Press `<enter>` or `<space>` to continue.
"""
        teach_provider("anthropic", "Anthropic", ["add", "default"], run_md,
                       "Anthropic connected.")
    elif pick.value == "Gemini":
        run_md = """\
Add your Gemini credentials. The key is stored securely and never
written to your config file.

1. In Rune's console, run `models providers gemini add default`.
2. Paste your API key at the redacted prompt.

Press `<enter>` or `<space>` to continue.
"""
        teach_provider("gemini", "Gemini", ["add", "default"], run_md,
                       "Gemini connected.")
    elif pick.value == "Codex":
        run_md = """\
Codex authenticates through your browser — no API key to paste.

1. In Rune's console, run `models providers codex login`.
2. Finish the sign-in in your browser.

Press `<enter>` or `<space>` to continue.
"""
        teach_provider("codex", "Codex", ["login"], run_md, "Codex connected.")
    else:
        run_md = """\
Claude signs in through your browser with your Claude Pro or Max
subscription. There is no API key to paste.

Agent activity through this provider draws from your Claude plan's
separate monthly Agent SDK credit, not your interactive usage limits.
Once that credit runs out, further usage bills at standard API rates if
you have usage credits enabled, and otherwise pauses until the credit
refreshes. The separate `anthropic` provider bills the same models by
API key instead.

1. In Rune's console, run `models providers claude login`.
2. Finish the sign-in in your browser.

Press `<enter>` or `<space>` to continue.
"""
        teach_provider("claude", "Claude", ["login"], run_md, "Claude connected.")

    floating_window(title = "Open the Rune Agent", text = agent_open_md,
                    dismiss_keys = [ck])
    wait_command(
        title    = "Open the Rune Agent",
        command  = "agent",
        on_error = ("Run `<cmd>agent` to start a conversation. Add an " +
                    "optional conversation name and model: " +
                    "`<cmd>agent <name> <model>`."),
    )
    notify(level = success, message = "Rune Agent is ready.")


def teach_cheatsheet():
    floating_window(title = "Your cheatsheet", text = cheatsheet_md,
                    alignment = "top",
                    dismiss_keys = dismiss_for("cheatsheet"))
    wait_command(
        title    = "Your cheatsheet",
        command  = "cheatsheet",
        on_error = "Run the `<cmd>cheatsheet` command to open your cheatsheet.",
    )
    notify(level = success, message = "That is your cheatsheet.")


def teach_help():
    floating_window(title = "One last thing", text = help_md,
                    alignment = "top",
                    dismiss_keys = dismiss_for("help"))
    wait_command(
        title    = "One last thing",
        command  = "help",
        on_error = "Run the `<cmd>help` command to open the docs and ask the help agent.",
    )
    notify(level = success, message = "That is the help command.")


def run():
    floating_window(
        title = "Welcome",
        text = welcome_md,
        allow_keys = [
            "<meta-1>", "<meta-2>", "<meta-3>",
            "<meta-4>", "<meta-5>", "<meta-6>",
            "<meta-7>", "<meta-8>", "<meta-9>",
            "<meta-enter>",
        ],
        dismiss_keys = [ck],
    )

    ws = wait_command(
        title    = "Welcome",
        command  = "workspaceopen",
        on_error = ("`<cmd>workspaceopen` needs a `<workspacepath>` " +
                    "argument. Use the auto-completer (Tab / arrow " +
                    "keys) to pick a workspace, or type a directory " +
                    "path (it will be created if it doesn't exist)."),
    )
    notify(level = success, message = "Opened workspace: " + ws.args[0])

    teach_edit()

    teach_layout()
    teach_split_window()
    teach_terminal()
    teach_modal_surfaces()
    teach_split_direction()
    teach_terminal_split()
    teach_focus_window()
    teach_move_window()
    teach_fullscreen_window()
    teach_close_window()
    teach_tabs()
    teach_switch_tabs()
    teach_close_tab()
    teach_terminals()

    teach_agent()

    teach_cheatsheet()

    teach_help()


tutorial(id = "basics", title = "Rune basics", version = "27", entry = run)
