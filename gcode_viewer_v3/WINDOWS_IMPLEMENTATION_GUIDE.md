# GcodeSim V3 Windows Implementation Guide

## Table of Contents
1. [Project Overview](#project-overview)
2. [Directory Structure](#directory-structure)
3. [Windows-Specific Features](#windows-specific-features)
4. [UI Components](#ui-components)
5. [Backend Architecture](#backend-architecture)
6. [Parser Implementation](#parser-implementation)
7. [Rendering System](#rendering-system)
8. [Build Process](#build-process)
9. [File Association](#file-association)
10. [Performance Optimizations](#performance-optimizations)

## Project Overview

GcodeSim V3 is a cross-platform 3D G-code viewer built with Go and the g3n engine. The Windows implementation includes special features for Windows users, including file association registration, native Windows build process, and platform-specific UI handling.

## Directory Structure

```
gcode_viewer_v3/
├── cmd/gcodesim/           # Main application entry point
│   └── main.go             # Entry point with Windows file association flags
├── internal/
│   ├── parser/             # G-code parsing functionality
│   ├── scene/              # 3D rendering components
│   └── ui/                 # User interface components
├── windows/                # Windows-specific build and configuration files
│   ├── build.bat           # Batch file wrapper for PowerShell build script
│   ├── build.ps1           # PowerShell build script with versioning
│   ├── versioninfo.json    # Windows version information metadata
│   └── icon.ico            # Application icon for Windows
└── mac/                    # macOS build scripts (for reference)
```

## Windows-Specific Features

### File Association Registration
The Windows version supports registering G-code file extensions to open with GcodeSim:

- **Extensions Supported**: .nc, .gcode, .ngc, .tap, .cnc, .gco, .g, .mpf, .nci, .tab, .eia, .dnc
- **Registration Method**: Uses Windows registry at HKCU\Software\Classes
- **Commands**: 
  - `gcodesim --register-file-types` - Register file associations
  - `gcodesim --unregister-file-types` - Remove file associations

### Build Process
The Windows build system uses PowerShell scripts to:
- Install required tools (goversioninfo)
- Generate Windows resource files with embedded icons and version info
- Build the executable with GUI subsystem flag (no console window)
- Embed version information via Go linker flags

## UI Components

### Toolbar (internal/ui/toolbar.go)
The toolbar consists of two rows:
- **Row 1**: Control buttons (Open, Play/Pause, Reset, Reframe) + Speed slider + Bit diameter input
- **Row 2**: Progress slider for scrubbing through G-code

Key features:
- Speed control with exponential mapping (0.5x to 50x)
- Bit diameter input with validation
- Material thickness setting with through-cut visualization
- Tutorials dropdown with bundled examples
- Options panel for advanced settings

### Window Management (internal/ui/window.go)
- Custom Z-up camera controller (Orbiter)
- View cube for 3D orientation
- Manual render loop with multi-pass rendering
- HiDPI display support
- Window resize handling
- Keyboard shortcuts (Ctrl+O, R, Space, Esc)

### File Dialogs (internal/ui/dialogs.go)
- Native Windows file dialogs
- Error message handling
- File type validation

## Backend Architecture

### Scene Graph (internal/scene/)
- **PathActor**: Renders toolpath lines with depth-based coloring
- **StockWireframe**: Displays stock outline
- **Tool**: 5-part end mill visualization (flute, helix, band, shank, LED)
- **Heightmap**: Material removal simulation using 2D grid
- **ViewCube**: Interactive 3D orientation indicator

### Parsing System (internal/parser/)
- **G-code Parser**: Handles modal G-code (G0/G1/G2/G3, M3/M5, G90/G91)
- **Arc Linearization**: Converts G2/G3 arcs to line segments using I/J center offsets
- **Bounds Calculation**: Computes min/max coordinates for framing
- **Depth Analysis**: Finds deepest cutting Z value

### Playback Engine (internal/scene/playback.go)
- **Time-based Motion**: Interpolates between moves using arc-length parameterization
- **Speed Control**: Adjustable playback speed with exponential mapping
- **Progress Tracking**: Sliding progress bar with scrub capability
- **Spindle Control**: Simulates spindle on/off state

### Rendering System

#### Heightmap Material Removal (internal/scene/removal.go)
- 2D grid system for material removal
- Flat shading via vertex duplication for crisp cut walls
- Through-cut visualization for material thickness
- Shell visualization for stock boundaries
- Performance-optimized refresh (15 Hz throttle)

#### View Cube (internal/scene/view_cube.go)
- Chamfered cube with 6 main octagonal faces + 8 corner triangles
- Hover highlighting for interactive feedback
- Click-to-snap functionality to standard views
- Perspective camera for correct picking behavior

## Build Process

### Windows Build Script (windows/build.ps1)
1. **Dependency Management**: Ensures goversioninfo is installed
2. **Module Synchronization**: Runs `go mod tidy`
3. **Resource Generation**: Creates Windows resource files with version info and icon
4. **Binary Build**: Compiles with:
   - `-H windowsgui` flag (GUI application)
   - `-s -w` flags (stripped symbols)
   - Git version injection for update checking

### Build Artifacts
- **Output**: `gcodesim.exe` in parent directory
- **Features**: Embedded icon, version information, no console window
- **Size**: ~5 MB executable

## File Association

### Registration Process
The application registers file associations by writing to:
- `HKCU\Software\Classes\Nightbots.GcodeFile` (ProgID)
- `HKCU\Software\Classes\.nc` → points to ProgID
- `HKCU\Software\Classes\.gcode` → points to ProgID
- etc. for all supported extensions

### Unregistration Process
Removes only associations that point to the current application, preserving user customizations.

## Performance Optimizations

### Rendering Optimizations
1. **Multi-pass Render**: Separate passes for main scene and view cube
2. **Layered Rendering**: Uses alpha bit planes for efficient transparency
3. **Flat Shading**: Vertex duplication for crisp cut walls
4. **Throttled Updates**: Heightmap refresh at 15 Hz cadence
5. **HiDPI Support**: Correct scaling on Retina displays

### Memory Management
1. **Vertex Buffer Optimization**: Efficient VBO usage with precomputed indices
2. **Mesh Reuse**: Reuses GPU mesh data structures
3. **Selective Updates**: Only redraws changed elements

### Playback Efficiency
1. **Move-by-move Cutting**: Precise arc-linearized cutting paths
2. **Progress Replay**: Replays cuts for slider scrubbing
3. **Performance Throttling**: 15 Hz mesh refresh rate

### File I/O
1. **Lazy Loading**: Builds scene components on demand
2. **Memory Efficient Parsing**: Minimal memory footprint during parsing
3. **Buffered Operations**: Optimized vertex buffer operations