(Tutorial 02: Pocket Clearing)
(=============================)
(What this does:)
(  Clears a 40 x 30 mm rectangular pocket 3 mm deep using parallel)
(  back-and-forth passes spaced 4 mm apart - typical raster pocket.)
(What to watch for in the simulator:)
(  - The carved surface deforms as the tool sweeps each row)
(  - Switching to TOP view (click TOP face on the view cube)) shows)
(    the pocket forming clearly from above)
(  - At the end, switching to FRONT or RIGHT view shows the clean)
(    flat-bottom pocket)
(Suggested settings:)
(  Bit diameter: 6 mm)
(  Material thickness: 0 leave default))
(  Speed: 5x is comfortable for watching the rows fill in)

G21
G90
G17
G0 Z5.0
M3 S12000

(Plunge to depth at the start of the first row)
G0 X10 Y10
G1 Z-3.0 F300

(Row 1: +X)
G1 X50 Y10 F1000

(Row 2: -X, stepped over by 4 mm)
G1 X50 Y14
G1 X10 Y14

(Row 3: +X)
G1 X10 Y18
G1 X50 Y18

(Row 4: -X)
G1 X50 Y22
G1 X10 Y22

(Row 5: +X)
G1 X10 Y26
G1 X50 Y26

(Row 6: -X)
G1 X50 Y30
G1 X10 Y30

(Row 7: +X)
G1 X10 Y34
G1 X50 Y34

(Row 8: -X final row)
G1 X50 Y38
G1 X10 Y38

(Outline pass to clean up the perimeter)
G1 X10 Y10
G1 X50 Y10
G1 X50 Y40
G1 X10 Y40
G1 X10 Y10

G0 Z5.0
G0 X0 Y0
M5
M30
