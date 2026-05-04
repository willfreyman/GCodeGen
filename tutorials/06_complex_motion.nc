(Tutorial 06: Mixed Operations)
(=============================)
(What this does:)
(  A small but realistic CAM program: clears a rectangular pocket,)
(  drills four holes at the corners of the pocket, slots between two)
(  pairs of holes, and finally cuts the outer profile.)
(What to watch for in the simulator:)
(  - Multiple operations stitched together with G0 rapids in between -)
(    the rapids are dashed pale-blue lines on the toolpath, distinct)
(    from the colored cuts)
(  - The "speed slider" stays sensible even with many G0/G1 transitions)
(  - At high speed 50x)) the swept-path cut walker keeps the heightmap)
(    accurate - no phantom cuts along the rapid lines between operations)
(  - Drag the progress bar back and forth - the cut/uncut state)
(    matches the tool position both ways)
(Suggested settings:)
(  Bit diameter: 6 mm)
(  Material thickness: 0 we're not cutting through))
(  Speed: try 50x to test the high-speed path correctness)

G21
G90
G17
G0 Z10.0
M3 S12000

(===== Op 1: rectangular pocket clear at -2 mm =====)
G0 X20 Y15
G1 Z-2.0 F300

G1 X60 Y15 F1000
G1 X60 Y20
G1 X20 Y20
G1 X20 Y25
G1 X60 Y25
G1 X60 Y30
G1 X20 Y30
G1 X20 Y35
G1 X60 Y35

(perimeter cleanup)
G1 X60 Y15
G1 X20 Y15

G0 Z10.0

(===== Op 2: drill four holes at the pocket corners =====)
(Hole 1)
G0 X20 Y15
G1 Z-5.0 F200
G0 Z10.0

(Hole 2)
G0 X60 Y15
G1 Z-5.0 F200
G0 Z10.0

(Hole 3)
G0 X60 Y35
G1 Z-5.0 F200
G0 Z10.0

(Hole 4)
G0 X20 Y35
G1 Z-5.0 F200
G0 Z10.0

(===== Op 3: connect holes with slots =====)
(Top slot: hole 1 to hole 2)
G0 X20 Y15
G1 Z-3.0 F300
G1 X60 Y15 F800
G0 Z10.0

(Bottom slot: hole 4 to hole 3)
G0 X20 Y35
G1 Z-3.0 F300
G1 X60 Y35 F800
G0 Z10.0

(===== Op 4: outer profile at -2 mm =====)
G0 X10 Y5
G1 Z-2.0 F300
G1 X70 Y5 F800
G1 X70 Y45
G1 X10 Y45
G1 X10 Y5

G0 Z10.0
G0 X0 Y0
M5
M30
