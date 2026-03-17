Require micasa

Output docs/static/videos/using-sorting.webm

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
Show

# Navigate to Status column (col 3).
Sleep 0.5s
Type "l"
Sleep 0.3s
Type "l"
Sleep 0.3s
Type "l"
Sleep 0.5s

# Sort ascending -- header shows sort indicator.
Type "s"
Sleep 1.5s

# Navigate to Type column (col 1).
Type "h"
Sleep 0.3s
Type "h"
Sleep 0.5s

# Add secondary sort -- both columns show priority numbers.
Type "s"
Sleep 1.5s

# Clear all sorts.
Type "S"
Sleep 1.5s

# Open fuzzy column finder.
Type "/"
Sleep 1s

# Type to filter columns.
Set TypingSpeed 80ms
Type "bud"
Set TypingSpeed 0
Sleep 1s

# Jump to Budget column.
Enter
Sleep 1.5s

# Hide column -- it disappears, badge appears below table.
Type "c"
Sleep 1.5s

# Show all columns again.
Type "C"
Sleep 1.5s

Hide
Ctrl+Q
Sleep 1s
