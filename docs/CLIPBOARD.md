# Copying diagnostics and the clipboard

Click a diagnostic value, result message, device detail, firmware reading or log
line to copy its exact underlying text. Clipping and wrapping only affect the
display. Values exclude field labels and borders; diagnostic messages retain
relevant measurements. `Tab` / `Shift+Tab` focuses copy targets, and `Enter` (or
`Space`) copies the focused target. Existing sliders and controller stage
interactions keep their controls.

Diagnostics, Device Information and Device Details expose **Copy All
Diagnostics**. Diagnostics exposes **Copy Current Test**; firmware exposes **Copy Test Results**.
`y` copies the selected diagnostic category (including drift/deadzone results),
or the firmware report on Device Details; `Y` copies the full diagnostic report.
With no category selected, `y` copies the summary. Reports use current application
state, include unsupported/unmeasured features and timestamps, and mark simulated
data. They contain no audio recordings or raw HID dumps. Unique identifiers are
excluded or redacted by default. The **Identifiers: Off/On** action explicitly
opts into including them; switching devices resets this choice. Selecting an
individual identifier copies that identifier directly.

Clipboard backends:

- Linux Wayland: `wl-copy` from `wl-clipboard`, preferred when `WAYLAND_DISPLAY`
  is set. In mixed sessions, a failed Wayland copy falls back to X11 tools.
- Linux X11: `xclip -selection clipboard -in`, then `xsel --clipboard --input`,
  when `DISPLAY` is set. Native command failures are reported, not shown as success.
- macOS: `pbcopy`.
- Windows: PowerShell `Set-Clipboard`, with UTF-8 standard input and error checking.
- SSH / terminal fallback: explicitly start with `TUISENSE_OSC52=1 tuisense` in a terminal that permits OSC 52 clipboard writes.
  The clipboard request is base64 encoded, with tmux passthrough when inside tmux.
  tmux must permit passthrough (`allow-passthrough on`), and the outer terminal
  must permit clipboard access. Terminal requests are limited to 64 KiB of UTF-8
  text and show “request sent; paste to verify”, since OSC 52 writes have no
  acknowledgement. SSH sessions skip remote desktop backends to target the local
  terminal clipboard. Plain stdout output is never treated as clipboard success.

If no backend works, the exact text is saved to a private (0600) temporary UTF-8
file. An accessible dialog shows the full path and setup instructions; `Enter`
or `Esc` closes it. Open the file with a text editor to copy/share its contents,
and delete it after use. Individual copies can include explicitly selected
identifiers, including in fallback files.

To verify locally, run `make build`, then `./bin/tuisense --mock`. Open Diagnostics,
click a result/value and paste into a text editor. Try the two report buttons,
`y` / `Y`, and `Tab` then `Enter`. In Advanced Device Details, click the value to
copy it or its label to show its explanation. Test privacy with a MAC/serial
reading before and after opting in. `go test ./...` includes clipboard backend,
report privacy, mouse/focus and existing controller interaction tests. Building
locally does not change the installed `tuisense` command.
