Require micasa

Output docs/static/videos/using-navigation.webm

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

# Browse rows on Projects tab.
Sleep 0.8s
Type "j"
Sleep 0.5s
Type "j"
Sleep 0.5s
Type "j"
Sleep 0.5s
Type "j"
Sleep 0.8s

# Browse columns.
Type "l"
Sleep 0.4s
Type "l"
Sleep 0.4s
Type "l"
Sleep 0.8s

# Switch to next tab.
Type "f"
Sleep 1.5s

# Switch to Maintenance tab.
Type "f"
Sleep 1.5s

# Navigate to Log drilldown column.
Type "l"
Sleep 0.3s
Type "l"
Sleep 0.3s
Type "l"
Sleep 0.3s
Type "l"
Sleep 0.3s
Type "l"
Sleep 0.8s

# Drilldown into service log.
Enter
Sleep 2s

# Close detail view.
Escape
Sleep 1.2s

# Enter Edit mode (status bar changes to orange EDIT).
Type "i"
Sleep 1.5s

# Return to Nav mode.
Escape
Sleep 1.2s

Hide
Ctrl+Q
Sleep 1s
