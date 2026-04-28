# Draw → G-code CNC Path Editor

A lightweight Tkinter-based tool for sketching toolpaths and exporting them directly to G-code for CNC machines.

## Features

* Draw freehand toolpaths on a canvas
* Adjustable **origin (0,0)** and **cutting perimeter**
* Multiple operations with:

  * Custom names
  * Individual cut depths
  * Color-coded paths
* Real-time **toolpath simulation**
* **Finished product preview** (material + bit diameter visualization)
* Export clean `.nc` G-code files
* Basic CNC parameter controls (feeds, RPM, safe Z)

## How It Works

1. Draw paths directly on the canvas using the left mouse button
2. Each stroke becomes a machining operation
3. Set:

   * Depth per operation
   * Perimeter size (mm)
   * Machine settings (feeds, RPM)
4. Simulate or preview
5. Export G-code

## Controls

### Canvas

* **Left click + drag** → draw toolpaths
* **Drag orange dot** → move origin (0,0)
* **Drag perimeter corners** → resize work area
* **Drag edges** → move entire perimeter

### Buttons

* `Snap 0,0` → sets origin to bottom-left of perimeter
* `Simulate Toolpath` → animated CNC path preview
* `Finished Product Preview` → visual cut result
* `Generate G-code` → export `.nc` file

## G-code Output

Generated code includes:

* Metric units (`G21`)
* Absolute positioning (`G90`)
* Safe Z moves
* Spindle start (`M3`)
* Per-operation toolpaths
* End commands (`M5`, `M30`)

## Requirements

* Python 3.x
* Tkinter (included with most Python installs)

## Run

```bash
python your_script.py
```

## File Reference

Main script:


## Limitations / Reality Check

* No collision detection — you can generate unsafe toolpaths
* No tool radius compensation — preview is visual, not CAM-accurate
* No arcs (G2/G3) — everything is linearized
* No machine-specific post-processing
* No validation against your CNC controller

If you run this blindly on a real machine, you can crash tools or damage hardware.

## Suggested Improvements

* Add tool diameter compensation (offset paths)
* Add grid snapping / straight-line mode
* Add arc detection (convert to G2/G3)
* Add machine profile presets (GRBL, Mach3, etc.)
* Add bounds checking before export

## License

Use at your own risk.
