//go:build js && wasm

// Command wasm exposes the primitive model to JavaScript for the web demo.
//
// The page runs one copy of this module per Web Worker, each holding an
// identical Model. A step mirrors Model.Step in the CLI, with the restarts
// that the CLI spreads over goroutines spread over Web Workers instead:
//
//  1. every worker runs primitiveSearch with its share of the restarts;
//  2. the page picks the lowest-energy result and sends it to every worker
//     with primitiveAdd, so all models stay identical;
//  3. if repeat > 0, the winning worker runs primitiveRepeat (the reduced
//     search Model.Step does for -rep) and the page adds those shapes to the
//     other workers.
//
// Shapes cross between workers as "TypeName:{json}" strings.
package main

import (
	"encoding/json"
	"fmt"
	"image"
	"reflect"
	"strings"
	"syscall/js"

	"github.com/fogleman/primitive/primitive"
)

var (
	model *primitive.Model
	// best is the state primitiveSearch found, kept for primitiveRepeat.
	best *primitive.State
)

var shapeTypes = map[string]func() primitive.Shape{
	"Triangle":         func() primitive.Shape { return &primitive.Triangle{} },
	"Rectangle":        func() primitive.Shape { return &primitive.Rectangle{} },
	"Ellipse":          func() primitive.Shape { return &primitive.Ellipse{} },
	"RotatedRectangle": func() primitive.Shape { return &primitive.RotatedRectangle{} },
	"Quadratic":        func() primitive.Shape { return &primitive.Quadratic{} },
	"RotatedEllipse":   func() primitive.Shape { return &primitive.RotatedEllipse{} },
	"Polygon":          func() primitive.Shape { return &primitive.Polygon{} },
}

func worker() *primitive.Worker { return model.Workers[0] }

// primitiveInit(rgba Uint8Array, w, h, bg) -> {background, score}
// An empty bg uses the average colour of the image, like the CLI.
func initModel(this js.Value, args []js.Value) any {
	w, h := args[1].Int(), args[2].Int()
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	js.CopyBytesToGo(im.Pix, args[0])

	var bg primitive.Color
	if hex := args[3].String(); hex == "" {
		bg = primitive.MakeColor(primitive.AverageImageColor(im))
	} else {
		bg = primitive.MakeHexColor(hex)
	}

	// The page draws the SVG, so the model's raster context stays at input
	// size rather than the CLI's 1024px default.
	model = primitive.NewModel(im, bg, max(w, h), 1)
	best = nil
	return map[string]any{
		"background": hexColor(bg),
		"score":      model.Score,
	}
}

// primitiveSearch(mode, alpha, n, age, m) -> {shape, alpha, energy, evaluated}
// Runs m random restarts of n candidates each, hill climbing each with age.
func search(this js.Value, args []js.Value) any {
	w := worker()
	w.Init(model.Current, model.Score)
	best = w.BestHillClimbState(primitive.ShapeType(args[0].Int()),
		args[1].Int(), args[2].Int(), args[3].Int(), args[4].Int())
	return map[string]any{
		"shape":     encode(best.Shape),
		"alpha":     best.Alpha,
		"energy":    best.Energy(),
		"evaluated": w.Counter,
	}
}

// primitiveAdd(shape, alpha) -> {svg, score}
func add(this js.Value, args []js.Value) any {
	shape, err := decode(args[0].String())
	if err != nil {
		panic(err)
	}
	return addShape(shape, args[1].Int())
}

// primitiveRepeat(repeat, age) -> [{shape, alpha, svg, score}]
// Must run on the worker whose search won, after its shape was added.
func repeat(this js.Value, args []js.Value) any {
	var added []any
	state := best
	for i := 0; i < args[0].Int(); i++ {
		state.Worker.Init(model.Current, model.Score)
		a := state.Energy()
		state = primitive.HillClimb(state, args[1].Int()).(*primitive.State)
		if a == state.Energy() {
			break
		}
		r := addShape(state.Shape, state.Alpha)
		r["shape"] = encode(state.Shape)
		r["alpha"] = state.Alpha
		added = append(added, r)
	}
	return added
}

func addShape(shape primitive.Shape, alpha int) map[string]any {
	model.Add(shape, alpha)
	i := len(model.Shapes) - 1
	c := model.Colors[i]
	attrs := fmt.Sprintf("fill=\"%s\" fill-opacity=\"%.3f\"", hexColor(c), float64(c.A)/255)
	return map[string]any{
		"svg":   model.Shapes[i].SVG(attrs),
		"score": model.Score,
	}
}

func encode(shape primitive.Shape) string {
	v := reflect.ValueOf(shape).Elem()
	c := reflect.New(v.Type()).Elem()
	c.Set(v)
	c.FieldByName("Worker").SetZero()
	b, err := json.Marshal(c.Addr().Interface())
	if err != nil {
		panic(err)
	}
	return v.Type().Name() + ":" + string(b)
}

func decode(s string) (primitive.Shape, error) {
	name, body, _ := strings.Cut(s, ":")
	newShape, ok := shapeTypes[name]
	if !ok {
		return nil, fmt.Errorf("unknown shape type %q", name)
	}
	shape := newShape()
	if err := json.Unmarshal([]byte(body), shape); err != nil {
		return nil, err
	}
	reflect.ValueOf(shape).Elem().FieldByName("Worker").Set(reflect.ValueOf(worker()))
	return shape, nil
}

func hexColor(c primitive.Color) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

func main() {
	js.Global().Set("primitiveInit", js.FuncOf(initModel))
	js.Global().Set("primitiveSearch", js.FuncOf(search))
	js.Global().Set("primitiveAdd", js.FuncOf(add))
	js.Global().Set("primitiveRepeat", js.FuncOf(repeat))
	select {}
}
