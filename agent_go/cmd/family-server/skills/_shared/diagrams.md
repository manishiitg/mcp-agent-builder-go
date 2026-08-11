# Diagrams — geometry, graphs, number lines

Read this whenever a page needs a **figure the child reads off**: an angle, a
circle, a triangle with labelled sides, a coordinate graph, a number line. For
photographs of real things (a landform, a map, an anatomical diagram) use
`find_image` instead — see html-design.md → "Real pictures".

## Never hand-write SVG coordinates for geometry

Use **JSXGraph**, which is already available on every page — declare the
geometry and it computes every coordinate itself. Do not compute points, arc
paths or label positions yourself.

This is not a style preference. Hand-authored SVG gets three things wrong
almost every time, and a wrong figure teaches the wrong thing:

- **SVG's y-axis points DOWN.** A point at angle θ on a circle is
  `cy − r·sin θ`, not `+`. Get the sign wrong and the whole figure is mirrored.
- **Arc paths** (`A rx ry rot large-arc sweep x y`) — the `large-arc` and
  `sweep` flags are easy to invert, and marking the angle at a vertex IS an arc.
- **Angle labels** belong on the bisector, offset outward — more trigonometry,
  and if it's off the label sits on top of a line.

JSXGraph does all of that from a declaration like "the angle at B".

## The pattern

Give the page a sized container and build the board on load:

```html
<div id="fig1" class="jxgbox" style="width:340px;height:280px"></div>
<script>
window.addEventListener('load', function () {
  var b = JXG.JSXGraph.initBoard('fig1', {
    boundingbox: [-1, 6, 9, -1],   // [left, top, right, bottom] — top > bottom
    axis: false, showNavigation: false, showCopyright: false,
    keepAspectRatio: true
  });
  // ... geometry here ...
});
</script>
```

- One `initBoard` per figure; give each container its own `id`.
- `keepAspectRatio: true` for anything geometric — without it a circle renders
  as an ellipse and a right angle stops looking right.
- `showNavigation`/`showCopyright` off: this is a teaching figure, not a
  sandbox to pan around.
- `boundingbox` is in MATH coordinates (y up, the normal way) — that is the
  whole point; you never touch pixels.
- Wrap in `load` so the library is ready.

Ids follow the page's own scheme (`fig1`, `fig2` …) so `open_file`'s `focus`
can scroll to a figure — see html-design.md.

## Angles

```js
var A = b.create('point', [1, 1], {name:'A', fixed:true, size:2});
var B = b.create('point', [5, 1], {name:'B', fixed:true, size:2});  // vertex
var C = b.create('point', [7, 4], {name:'C', fixed:true, size:2});
b.create('segment', [B, A]);
b.create('segment', [B, C]);
// The arc, the label and the degree value — all computed for you:
b.create('angle', [C, B, A], {radius:1, name:'&ang;ABC'});
```

- **The vertex is the MIDDLE point** in `['angle', [P, Q, R]]` — the angle at Q.
  Naming a three-letter angle any other way is the classic error.
- **Point ORDER decides which side is marked, and it is easy to get backwards.**
  The arc sweeps ANTICLOCKWISE from the first point to the third, so the wrong
  order shades the reflex angle instead of the one you meant — verified: with
  A(1,1), B(5,1), C(7,4), writing `['angle', [A, B, C]]` shades the ~236°
  reflex side, and `['angle', [C, B, A]]` shades the ~124° angle actually
  being taught. Both look plausible in the code; only one is right on screen.
  This is precisely why you look at the rendered figure before finishing — if
  the shaded region is the big one going the long way round, swap the first and
  third points.
- For a right angle, JSXGraph draws the square marker automatically when the
  angle is 90°.
- To show the measured value instead of a name, use
  `{name: function(){ return JXG.Math.Geometry.trueAngle(A,B,C).toFixed(0)+'°'; }}`.
- Always use `°` in any text you write about it.

## Circles

```js
var O = b.create('point', [4, 3], {name:'O', fixed:true, size:2});
var c = b.create('circle', [O, 2.5], {strokeWidth:2});          // centre + radius
var P = b.create('glider', [6.5, 3, c], {name:'P', fixed:true}); // a point ON it
b.create('segment', [O, P], {dash:2, name:'r', withLabel:true});
```

- `['circle', [centre, radius]]` or `['circle', [centre, pointOnIt]]`.
- A point that must sit exactly on the circle is a **glider** — never a plain
  point at coordinates you worked out, which will be slightly off.
- Arcs/sectors: `b.create('sector', [O, P, Q])`, `b.create('arc', [O, P, Q])` —
  again, no path flags to get wrong.

## Graphs and number lines

```js
// A function graph — axes on, aspect ratio free:
var b = JXG.JSXGraph.initBoard('fig2', {
  boundingbox: [-5, 12, 5, -3], axis: true,
  showNavigation:false, showCopyright:false
});
b.create('functiongraph', [function (x) { return x * x; }], {strokeWidth:2});
```

For a **bar chart** whose values the child reads off, use real data and let the
chart place the bars:

```js
b.create('chart', [[1,2,3,4], [7,4,9,5]], {chartStyle:'bar', width:0.6});
```

- Bars sharing a figure share one scale. Never two scales in one chart.
- Label the axes, and give the figure a title in the surrounding `<figcaption>`,
  not inside the board.
- **Never invent a number to make a bar** — same rule as html-design.md.

If the child is meant to DRAW the graph herself, don't render it: give her the
empty grid (`axis:true`, nothing plotted) and let the question ask for title,
labels, scale and equal-width bars.

## Styling

Match the page: pass `{strokeColor:'…', fillColor:'…'}` using the page's own
CSS variables' literal values. Keep figures uncluttered — label only what the
question refers to. `fixed: true` on every point unless the child is genuinely
meant to drag it (a static worksheet figure should not move under her).

## Checking it

A figure that renders wrong is worse than no figure. After writing a page that
contains one, LOOK at it before you finish — see html-design.md → "Check a
figure before you finish".
