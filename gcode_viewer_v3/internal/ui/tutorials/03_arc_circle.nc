(Tutorial 03: Arcs and Circles)
(==============================)
(What this does:)
(  Cuts a 40 mm diameter circle at -2 mm depth using two G2 half-arcs)
(  joined at the X-axis. Then a smaller G3 inner circle.)
(What to watch for in the simulator:)
(  - Smooth curves on the toolpath - the parser linearizes G2/G3 arcs)
(    into ~16-32 segments per quarter-turn so the rendered path follows)
(    the true arc, not chord shortcuts)
(  - The carved surface is round, not faceted - confirms the cut math)
(    samples each linearized segment correctly)
(  - G3 in the inner circle shows the OPPOSITE arc direction - if the)
(    sim ever drew the chord between endpoints instead of the curve,)
(    you'd see this go the wrong way)
(Suggested settings:)
(  Bit diameter: 4 mm a smaller bit shows the circle outline more crisply))
(  Material thickness: 0)

G21
G90
G17
G0 Z5.0
M3 S12000

(Outer circle: center at 30,30, radius 20, cut clockwise via two G2s)
G0 X50 Y30      (start at +X side of circle)
G1 Z-2.0 F300

G2 X10 Y30 I-20 J0 F800   (top half: clockwise from +X to -X)
G2 X50 Y30 I20 J0         (bottom half: clockwise from -X to +X)

(Lift, move to inner-circle start)
G0 Z5.0
G0 X40 Y30      (inner circle radius 10, +X side)
G1 Z-2.0 F300

G3 X20 Y30 I-10 J0 F800   (counter-clockwise top half)
G3 X40 Y30 I10 J0         (counter-clockwise bottom half)

G0 Z5.0
G0 X0 Y0
M5
M30
