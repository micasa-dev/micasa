Require micasa

Output docs/static/videos/using-undo-redo.webm

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
# Disable XON/XOFF flow control so Ctrl+S reaches the app.
Type "stty -ixon"
Enter
Sleep 0.5s
Type "exec micasa --demo"
Enter
Sleep 5s
# Dismiss dashboard.
Type "D"
Sleep 2s
Show

# Enter Edit mode -- status bar shows orange EDIT badge.
Sleep 0.5s
Type "i"
Sleep 1.5s

# Navigate to Status column (col 3: ID=0, Type=1, Title=2, Status=3).
Type "l"
Sleep 0.3s
Type "l"
Sleep 0.3s
Type "l"
Sleep 0.8s

# Inline edit -- select form overlay appears.
Type "e"
Sleep 3s

# Navigate to a different option in the select.
Down
Sleep 0.5s
Down
Sleep 0.5s

# Confirm selection with Enter.
Enter
Sleep 2.5s

# Undo -- status reverts to original.
Type "u"
Sleep 2.5s

# Redo -- new status returns.
Type "r"
Sleep 2.5s

Hide
Ctrl+Q
Sleep 1s
