(Test fixture for parser golden tests.)
(Exercises: G0 rapid, G1 cut, G2/G3 arcs, M3/M5, axis carry-over, comments.)
G21
G90
G17
G0 Z5.0
M3 S12000
G0 X0 Y0
G0 Z1.0
G1 Z-2.0 F300
G1 X10.0 Y0.0 F800
G1 X10.0 Y10.0
G1 X0.0 Y10.0
G1 X0.0 Y0.0
G2 X10.0 Y0.0 I5.0 J0.0 ; semicolon comment
G3 X0.0 Y0.0 I-5.0 J0.0
G0 Z5.0
G0 X0 Y0
M5
M30
