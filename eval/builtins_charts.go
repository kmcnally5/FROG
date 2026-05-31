//go:build !js

package eval

// lineChart, barChart, sparkline, pieChart all moved to
// eval/builtins_ui_widgets.go (Phase 1b). They render via the shared
// uiRenderer interface (drawLine / fillPolygon / fillArc / fillRoundedRect)
// so the same widget bodies work on desktop GL and browser Canvas2D.
//
// This file is intentionally left tiny; once another !js-only chart
// surface needs a home, it can land here.
