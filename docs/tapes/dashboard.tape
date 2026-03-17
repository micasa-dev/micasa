Require micasa

Output docs/static/videos/dashboard.webm

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

# Show the dashboard.
Sleep 2s

# Navigate down through items.
Type "j"
Sleep 0.4s
Type "j"
Sleep 0.4s
Type "j"
Sleep 0.4s
Type "j"
Sleep 0.4s
Type "j"
Sleep 0.8s

# Jump to next section.
Type "J"
Sleep 1s

# Jump to next section again.
Type "J"
Sleep 1s

# Collapse current section.
Type "e"
Sleep 1s

# Expand it again.
Type "e"
Sleep 1s

# Jump to first item.
Type "g"
Sleep 1s

# Jump to last item.
Type "G"
Sleep 1s

# Navigate back up.
Type "k"
Sleep 0.4s
Type "k"
Sleep 0.4s
Type "k"
Sleep 0.8s

# Press enter to jump to the highlighted item's tab.
Enter
Sleep 2s

Hide
Ctrl+Q
Sleep 1s
