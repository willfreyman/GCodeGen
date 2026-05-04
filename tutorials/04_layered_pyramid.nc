(Tutorial 04: Layered Pyramid)
(============================)
(What this does:)
(  Cuts a stepped pyramid by tracing three concentric squares, each)
(  one smaller and one mm deeper than the last. Final step is a 4 mm)
(  square at -3 mm depth, with two outer steps at -1 and -2 mm.)
(What to watch for in the simulator:)
(  - The toolpath color shifts from green/yellow at -1 mm to)
(    orange/red at -2 mm to red/purple at -3 mm via the depth gradient)
(  - The carved surface forms three terraces, like a stepped pyramid)
(    seen from above)
(  - SIDE view FRONT or RIGHT face on the cube)) shows the steps)
(    clearly in profile)
(Suggested settings:)
(  Bit diameter: 3 mm smaller bit makes the steps cleaner))
(  Material thickness: 0)
(  Speed: 1-2x to watch each pass; 10x+ to see the final shape fast)

G21
G90
G17
G0 Z5.0
M3 S12000

(Step 1: outer square, 30x30 at -1 mm)
G0 X10 Y10
G1 Z-1.0 F300
G1 X40 Y10 F800
G1 X40 Y40
G1 X10 Y40
G1 X10 Y10

(Lift, move in for next step)
G0 Z5.0
G0 X15 Y15

(Step 2: middle square, 20x20 at -2 mm)
G1 Z-2.0 F300
G1 X35 Y15 F800
G1 X35 Y35
G1 X15 Y35
G1 X15 Y15

(Lift, move in for innermost step)
G0 Z5.0
G0 X20 Y20

(Step 3: innermost square, 10x10 at -3 mm)
G1 Z-3.0 F300
G1 X30 Y20 F800
G1 X30 Y30
G1 X20 Y30
G1 X20 Y20

G0 Z5.0
G0 X0 Y0
M5
M30
