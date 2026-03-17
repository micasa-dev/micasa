Require micasa

Output docs/static/images/ops-tree.webm

Set Shell bash
Set FontFamily "Hack Nerd Font"
Set FontSize 32
Set Width 2400
Set Height 1200
Set Padding 20
Set Theme "Dracula"
Set CursorBlink false
Set TypingSpeed 0

Env NO_COLOR ""
Env TERM "xterm-256color"
Env COLORTERM "truecolor"
Env COLORFGBG "15;0"
Env PS1 ""

Hide
Type "exec micasa demo"
Enter
Sleep 5s
# Dismiss dashboard.
Type "D"
Sleep 1s
# Navigate to Documents tab (6 f presses).
Type "f"
Sleep 0.3s
Type "f"
Sleep 0.3s
Type "f"
Sleep 0.3s
Type "f"
Sleep 0.3s
Type "f"
Sleep 0.3s
Type "f"
Sleep 1s
# Navigate to Ops column (6 l presses from ID).
Type "l"
Sleep 0.2s
Type "l"
Sleep 0.2s
Type "l"
Sleep 0.2s
Type "l"
Sleep 0.2s
Type "l"
Sleep 0.2s
Type "l"
Sleep 1s
# Open ops tree overlay.
Enter
Sleep 1s
Show

Sleep 1s

# Expand some nodes for a richer view.
Type "l"
Sleep 0.5s
Type "j"
Sleep 0.3s
Type "l"
Sleep 0.5s
Type "j"
Sleep 0.3s
Type "j"
Sleep 0.3s
Type "l"
Sleep 0.5s

Sleep 2s

Hide
Ctrl+Q
Sleep 1s
