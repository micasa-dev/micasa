Require micasa

Output docs/static/videos/using-date-input.webm

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
Sleep 2s
# Navigate to Start date column (col 6).
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
Sleep 0.5s
# Move to a row that has a start date.
Type "j"
Sleep 0.3s
Type "j"
Sleep 0.5s
Show

# Enter Edit mode.
Type "i"
Sleep 1s

# Inline edit the date cell -- opens calendar picker.
Type "e"
Sleep 1.5s

# Navigate days with h/l.
Type "l"
Sleep 0.5s
Type "l"
Sleep 0.5s
Type "l"
Sleep 0.5s

# Navigate weeks with j/k.
Type "j"
Sleep 0.5s
Type "j"
Sleep 0.5s
Type "k"
Sleep 0.5s

# Jump months with H/L.
Type "L"
Sleep 1s
Type "L"
Sleep 1s
Type "H"
Sleep 1s

# Jump to today with t.
Type "t"
Sleep 1.5s

# Pick the date with enter.
Enter
Sleep 1.5s

Hide
Ctrl+Q
Sleep 1s
