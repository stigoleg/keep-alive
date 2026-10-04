# TODO(2.0.0): re-record for the v2 interactive UI. These steps drive the 1.x
# menu (j to move, a number to pick a duration) and no longer match; the
# README shows a text screenshot until a new demo.gif exists. Render it from
# docs/ with `vhs demo.tape`, then add the image back to README.md.

Output demo.gif

Require echo

Set Framerate 60
Set TypingSpeed 0.1
Set PlaybackSpeed 1
Set Shell "bash"
Set FontSize 26
Set Width 1200
Set Height 600

Sleep 1500ms
Type "keepalive"
Sleep 2s
Enter
Sleep 4s
Enter
Sleep 5s
Enter
Sleep 1s
Type "j"
Sleep 1s
Enter
Sleep 1s
Type "1"
Sleep 500ms
Enter
Sleep 15s
Enter
Sleep 1s
Type "j"
Sleep 1s
Enter

