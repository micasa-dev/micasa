Require micasa

Output videos/demo.webm

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
# Dismiss dashboard
Type "D"
Sleep 2s
Show
Sleep 0.5s

# Projects tab -- browse rows
Sleep 1.5s
Type "j"
Sleep 0.6s
Type "j"
Sleep 0.6s
Type "j"
Sleep 0.6s
Type "j"
Sleep 1s

# Move to a column and sort
Type "l"
Sleep 0.5s
Type "l"
Sleep 0.5s
Type "s"
Sleep 1.2s
Type "s"
Sleep 1.5s

# Maintenance tab
Type "f"
Sleep 2s
Type "j"
Sleep 0.5s
Type "j"
Sleep 0.5s

# Navigate to Log column
Type "l"
Sleep 0.4s
Type "l"
Sleep 0.4s
Type "l"
Sleep 0.4s
Type "l"
Sleep 0.4s
Type "l"
Sleep 1s

# Drilldown into service log
Enter
Sleep 3s
Escape
Sleep 1.5s

# Incidents tab
Type "f"
Sleep 2s
Type "j"
Sleep 0.5s
Type "j"
Sleep 0.5s
Type "j"
Sleep 1s

# Appliances tab
Type "f"
Sleep 2s
Type "j"
Sleep 0.5s
Type "j"
Sleep 0.5s
Type "j"
Sleep 1s

# House profile toggle
Tab
Sleep 3s
Tab
Sleep 1.5s

# Help overlay
Type "?"
Sleep 4s
Escape
Sleep 1s

# Dashboard
Type "D"
Sleep 3s
Type "D"
Sleep 3s

# Quit (hidden so the post-exit terminal doesn't leak into the recording)
Hide
Ctrl+Q
Sleep 1s
