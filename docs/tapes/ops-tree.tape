Require micasa

Output docs/static/videos/ops-tree.webm

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
Show

Sleep 0.5s

# Open ops tree overlay on the Invoice row.
Enter
Sleep 1.5s

# Navigate down through the tree, expanding as we go.
# Expand first operation.
Type "l"
Sleep 0.8s
# Move into action, table, then data.
Type "j"
Sleep 0.4s
Type "j"
Sleep 0.4s
Type "j"
Sleep 0.4s
# Expand data node.
Type "l"
Sleep 0.8s
# Browse the vendor fields.
Type "j"
Sleep 0.4s
Type "j"
Sleep 0.4s
Type "j"
Sleep 0.4s

Sleep 1s

# Move to second operation.
Type "j"
Sleep 0.4s
# Expand it.
Type "l"
Sleep 0.8s
Type "j"
Sleep 0.4s
Type "j"
Sleep 0.4s
Type "j"
Sleep 0.4s
# Expand data.
Type "l"
Sleep 0.8s
Type "j"
Sleep 0.4s
Type "j"
Sleep 0.4s

Sleep 1s

# Move to third operation and expand.
Type "j"
Sleep 0.4s
Type "l"
Sleep 0.8s
Type "j"
Sleep 0.4s
Type "j"
Sleep 0.4s
Type "j"
Sleep 0.4s
Type "l"
Sleep 0.8s

Sleep 1.5s

# Collapse everything back up -- jump to top and collapse root.
Type "g"
Sleep 0.5s
Type "h"
Sleep 1s

# Re-expand to show the toggle.
Type "l"
Sleep 1s

Sleep 1.5s

# Close overlay.
Escape
Sleep 1s

Hide
Ctrl+Q
Sleep 1s
