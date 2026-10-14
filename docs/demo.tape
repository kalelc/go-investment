# Regenerate with: make screenshots
Output docs/demo.gif

Require ./bin/goinv

Set Shell "bash"
Set FontSize 15
Set Width 1400
Set Height 950
Set Padding 20
Set Theme "Catppuccin Mocha"
Set TypingSpeed 60ms

Hide
Type "./bin/goinv demo" Enter
Sleep 3s
Show

Sleep 1.5s
Screenshot docs/panel.png

Down Sleep 600ms
Down Sleep 600ms
Up Sleep 600ms
Up Sleep 600ms
Enter
Sleep 2s
Screenshot docs/mensual.png

Down Sleep 400ms
Down Sleep 400ms
Up Sleep 400ms
Up Sleep 400ms
Tab
Sleep 2s
Screenshot docs/diario.png

Escape
Sleep 1.5s
Type "q"
Sleep 500ms
