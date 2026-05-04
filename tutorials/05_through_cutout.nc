(Tutorial 05: Through-Cut Profile)
(=================================)
(What this does:)
(  Cuts a 30 x 30 mm square contour all the way through 6 mm of)
(  material. Final cut depth is -6.5 mm so the tool clears the bottom.)
(What to watch for in the simulator:)
(  - With Material Thickness UNSET 0)) the cuts just deform the surface)
(    down to -6.5 mm - no actual hole through)
(  - With Material Thickness set to 6 mm in the Options panel and)
(    after clicking Reset)) the inner square actually SLICES OUT - you)
(    see real holes in the mesh where the cell were eroded past -6 mm)
(  - This is exactly the "cut a part out of stock" workflow real CNC)
(    routers do - the inside square would fall out as a finished part)
(Suggested settings:)
(  Bit diameter: 3 mm)
(  Material thickness: 6 mm  <-- this is the whole point of the tutorial)
(  Speed: 10x is fine; the cut count is small)

G21
G90
G17
G0 Z5.0
M3 S12000

(First pass at -2 mm to break through the surface)
G0 X20 Y20
G1 Z-2.0 F300
G1 X50 Y20 F800
G1 X50 Y50
G1 X20 Y50
G1 X20 Y20

(Second pass at -4 mm)
G1 Z-4.0 F300
G1 X50 Y20 F800
G1 X50 Y50
G1 X20 Y50
G1 X20 Y20

(Final pass at -6.5 mm - through the stock and a bit past)
G1 Z-6.5 F300
G1 X50 Y20 F800
G1 X50 Y50
G1 X20 Y50
G1 X20 Y20

G0 Z5.0
G0 X0 Y0
M5
M30
