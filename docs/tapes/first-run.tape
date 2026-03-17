Require micasa

Output docs/static/videos/first-run.webm

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
Show

# Show the dashboard for a moment.
Sleep 3s

# Dismiss dashboard and land on the Projects tab.
Type "f"
Sleep 2s

# Navigate between tabs.
Type "f"
Sleep 1s
Type "f"
Sleep 1s
Type "f"
Sleep 1s

# Go back to first tab.
Type "B"
Sleep 1.5s

# Move around the table.
Type "j"
Sleep 0.3s
Type "j"
Sleep 0.3s
Type "j"
Sleep 0.3s
Type "l"
Sleep 0.3s
Type "l"
Sleep 0.3s
Type "l"
Sleep 1.5s

# Reopen dashboard.
Type "D"
Sleep 2s

Hide
Ctrl+Q
Sleep 1s
