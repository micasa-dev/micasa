Require micasa

Output docs/static/videos/using-filtering.webm

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
Type "exec micasa --demo"
Enter
Sleep 5s
# Dismiss dashboard.
Type "D"
Sleep 2s
Show

# Navigate to Status column (col 3).
Sleep 0.5s
Type "l"
Sleep 0.3s
Type "l"
Sleep 0.3s
Type "l"
Sleep 0.8s

# Move down to a row.
Type "j"
Sleep 0.4s
Type "j"
Sleep 0.5s

# Pin current cell value -- non-matching rows dim.
Type "n"
Sleep 2s

# Move to another row and pin a different value in the same column (OR logic).
Type "j"
Sleep 0.4s
Type "j"
Sleep 0.4s
Type "j"
Sleep 0.5s
Type "n"
Sleep 2s

# Activate filter -- non-matching rows disappear.
Type "N"
Sleep 2s

# Clear all pins and deactivate.
Ctrl+N
Sleep 1.5s

Hide
Ctrl+Q
Sleep 1s
