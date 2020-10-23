package graphic

import (
	"context"
	"errors"
	"math"
	"sync/atomic"

	"github.com/noriah/tavis/util"

	"github.com/nsf/termbox-go"
)

const (

	// MaxWidth will be removed at some point
	MaxWidth = 5000

	// DrawCenterSpaces is tmp
	DrawCenterSpaces = false

	// DrawPaddingSpaces do we draw the outside padded spacing?
	DrawPaddingSpaces = false

	// DisplaySpace is the block we use for space (if we were to print one)
	DisplaySpace rune = '\u0020'

	// NumRunes number of runes for sub step bars
	NumRunes = 8

	// Scaling Constants

	// ScalingSlowWindow in seconds
	ScalingSlowWindow = 5

	// ScalingFastWindow in seconds
	ScalingFastWindow = ScalingSlowWindow * 0.2

	// ScalingDumpPercent is how much we erase on rescale
	ScalingDumpPercent = 0.75

	// ScalingResetDeviation standard deviations from the mean before reset
	ScalingResetDeviation = 1
)

// DrawType is the type
type DrawType int

// Draw Types
const (
	DrawMin DrawType = iota
	DrawUp
	DrawUpDown
	DrawDown
	DrawMax
)

// DrawDefault is the default draw type
const DrawDefault = DrawUpDown

// Bar runes
const (
	BarRuneR = '\u2580'
	BarRune  = '\u2588'
)

// Temp styles
const (
	StyleDefault     = termbox.ColorDefault
	StyleDefaultBack = termbox.ColorDefault
	StyleCenter      = termbox.ColorMagenta
	// StyleCenter  = StyleDefault
	StyleReverse = termbox.AttrReverse
)

// Config is a Display Config Object
type Config struct {
	BarWidth   int
	SpaceWidth int
	BinWidth   int
	BaseThick  int
	DrawType   DrawType
}

// Display handles drawing our visualizer
type Display struct {
	running uint32

	cfg Config

	slowWindow *util.MovingWindow
	fastWindow *util.MovingWindow
}

// NewDisplay sets up a new display
// should we panic or return an error as well?
// something to think about
func NewDisplay(hz float64, samples int) *Display {

	if err := termbox.Init(); err != nil {
		panic(err)
	}

	termbox.SetInputMode(termbox.InputAlt)
	termbox.SetOutputMode(termbox.Output256)

	termbox.HideCursor()

	slowMax := int((ScalingSlowWindow*hz)/float64(samples)) * 2
	fastMax := int((ScalingFastWindow*hz)/float64(samples)) * 2

	var d = &Display{
		slowWindow: util.NewMovingWindow(slowMax),
		fastWindow: util.NewMovingWindow(fastMax),
		cfg: Config{
			BarWidth:   2,
			SpaceWidth: 1,
			BinWidth:   3,
			BaseThick:  1,
			DrawType:   DrawDefault,
		},
	}

	return d
}

// Start display is bad
func (d *Display) Start(ctx context.Context) context.Context {

	var dispCtx, dispCancel = context.WithCancel(ctx)
	go eventPoller(dispCtx, dispCancel, d)
	return dispCtx
}

// eventPoller will take events and do things with them
// TODO(noraih): MAKE THIS MORE ROBUST LIKE PREGO TOMATO SAUCE LEVELS OF ROBUST
func eventPoller(ctx context.Context, fn context.CancelFunc, d *Display) {
	defer fn()

	atomic.StoreUint32(&d.running, 1)
	defer atomic.StoreUint32(&d.running, 0)

	for {
		// first check if we need to exit
		select {
		case <-ctx.Done():
			return
		default:
		}

		var ev = termbox.PollEvent()

		switch ev.Type {
		case termbox.EventKey:
			switch ev.Key {

			case termbox.KeyArrowUp:
				d.SetWidths(d.cfg.BarWidth+1, d.cfg.SpaceWidth)

			case termbox.KeyArrowRight:
				d.SetWidths(d.cfg.BarWidth, d.cfg.SpaceWidth+1)

			case termbox.KeyArrowDown:
				d.SetWidths(d.cfg.BarWidth-1, d.cfg.SpaceWidth)

			case termbox.KeyArrowLeft:
				d.SetWidths(d.cfg.BarWidth, d.cfg.SpaceWidth-1)

			case termbox.KeySpace:
				d.SetDrawType(d.cfg.DrawType + 1)

			case termbox.KeyCtrlC:
				return
			default:

				switch ev.Ch {
				case '+', '=':
					d.SetBase(d.cfg.BaseThick + 1)

				case '-', '_':
					d.SetBase(d.cfg.BaseThick - 1)

				case 'q', 'Q':
					return

				default:

				} // switch ev.Ch

			} // switch ev.Key

		case termbox.EventInterrupt:
			return

		default:

		} // switch ev.Type
	}
}

// Stop display not work
func (d *Display) Stop() error {
	if atomic.CompareAndSwapUint32(&d.running, 1, 0) {
		termbox.Interrupt()
	}

	return nil
}

// Close will stop display and clean up the terminal
func (d *Display) Close() error {
	termbox.Close()
	return nil
}

// SetWidths takes a bar width and spacing width
// Returns number of bars able to show
func (d *Display) SetWidths(bar, space int) {
	if bar < 1 {
		bar = 1
	}

	if space < 0 {
		space = 0
	}

	d.cfg.BarWidth = bar
	d.cfg.SpaceWidth = space
	d.cfg.BinWidth = bar + space
}

// SetBase will set the base thickness
func (d *Display) SetBase(thick int) {
	if thick < 0 {
		thick = 0
	}

	d.cfg.BaseThick = thick
}

// SetDrawType sets the draw type for future draws
func (d *Display) SetDrawType(dt DrawType) {
	switch {
	case dt <= DrawMin:
		dt = DrawMax - 1
	case dt >= DrawMax:
		dt = DrawMin + 1
	}

	d.cfg.DrawType = dt
}

// Bars returns the number of bars we will draw
func (d *Display) Bars(sets ...int) int {
	var x = 1
	if len(sets) > 0 {
		x = sets[0]
	}

	var width, _ = termbox.Size()

	switch d.cfg.DrawType {
	case DrawUpDown:
		return width / d.cfg.BinWidth
	case DrawUp, DrawDown:
		return (width / d.cfg.BinWidth) / x
	default:
		return -1
	}
}

func (d *Display) updateWindow(peak float64) float64 {
	// do some scaling if we are above 0
	if peak > 0.0 {
		d.fastWindow.Update(peak)
		var vMean, vSD = d.slowWindow.Update(peak)

		if length := d.slowWindow.Len(); length >= d.fastWindow.Cap() {

			if math.Abs(d.fastWindow.Mean()-vMean) > (ScalingResetDeviation * vSD) {
				vMean, vSD = d.slowWindow.Drop(
					int(float64(length) * ScalingDumpPercent))
			}
		}

		return math.Max(vMean+(2.5*vSD), 1.0)
	}

	return 1
}

// Draw takes data and draws
func (d *Display) Draw(bins [][]float64, count int) error {

	switch len(bins) {
	case 1, 2:
	default:
		return errors.New("bad number of sets to draw")
	}

	var peak = 0.0

	for xSet := 0; xSet < len(bins); xSet++ {
		for xBin := 0; xBin < count; xBin++ {
			if peak < bins[xSet][xBin] {
				peak = bins[xSet][xBin]
			}
		}
	}

	var scale = d.updateWindow(peak)

	var err error

	switch d.cfg.DrawType {
	case DrawUp:
		err = drawUp(bins, count, d.cfg, scale)
	case DrawUpDown:
		err = drawUpDown(bins, count, d.cfg, scale)
	case DrawDown:
		err = drawDown(bins, count, d.cfg, scale)
	default:
		return nil
	}

	if err != nil {
		return err
	}

	termbox.Flush()

	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)

	return nil
}

func stopAndTop(value float64, height int, up bool) (int, rune) {
	if stop := int(value * NumRunes); stop < height*NumRunes {
		if up {
			top := BarRuneR + rune(stop%NumRunes)
			stop = height - (stop / NumRunes)

			return stop, top
		}

		top := BarRune - rune(stop%NumRunes)
		stop /= NumRunes

		return stop, top
	}

	if up {
		return 0, BarRune
	}

	return height, BarRune
}