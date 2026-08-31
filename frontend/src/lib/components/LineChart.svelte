<script module lang="ts">
	// Gradient ids must be unique across the whole document (SVG ids aren't
	// component-scoped) — a shared counter gives each chart instance its own
	// prefix, avoiding collisions when multiple charts render on one page.
	let chartInstanceCounter = 0;
	function nextInstanceId(): number {
		return chartInstanceCounter++;
	}
</script>

<script lang="ts">
	// Shared chart primitive per design-wff-ui-konzept (#582/#145): labeled
	// axes, a legend when there's more than one series, and a hover tooltip
	// with the exact value. Separation is via depth (shadow/elevation) and
	// soft color washes, not gridlines or borders — no <line> gridnet, no
	// dashed hover guide, no bordered tooltip box.
	interface Series {
		name: string;
		color: string;
		values: (number | null)[];
		// Shown as a native tooltip on the legend entry — cheapest way to
		// explain jargon (CTL/ATL/TSB) without a custom tooltip component.
		description?: string;
	}

	let {
		xValues,
		series,
		xFormat,
		yFormat,
		ariaLabel,
		maxWidth = 800,
		height = 220,
		baseline,
		bare = false
	}: {
		xValues: number[];
		series: Series[];
		xFormat: (x: number) => string;
		yFormat: (y: number) => string;
		ariaLabel: string;
		maxWidth?: number;
		height?: number;
		// A reference line ("Frische über null ist erholt", a rider's own FTP)
		// drawn once at this y-value — dashed so it reads as a reference, never
		// as a fourth series (Nocturne v2).
		baseline?: number;
		// Suppresses axes/legend/tooltip/padding for a small full-bleed strip
		// (the ride-detail header's elevation profile) where the shape alone is
		// the point, not the exact numbers (Nocturne v2).
		bare?: boolean;
	} = $props();

	const instanceId = nextInstanceId();

	// The viewBox tracks the measured width so one SVG unit is one CSS pixel.
	// It used to be a fixed 800 stretched to 100 %, which on a 390 px phone
	// scaled everything down by more than half — an 11px axis label rendered
	// at 5px and was simply unreadable (#605). Nothing inside scales now.
	let measuredWidth = $state(0);
	let width = $derived(Math.min(measuredWidth || maxWidth, maxWidth));

	// A phone can't fit five date labels without them colliding.
	let narrow = $derived(width < 480);
	let padRight = $derived(bare ? 0 : 12);
	let padTop = $derived(bare ? 2 : 12);
	let padBottom = $derived(bare ? 2 : narrow ? 24 : 28);

	function niceTicks(min: number, max: number, count: number): number[] {
		if (min === max) return [min];
		const step = (max - min) / (count - 1);
		return Array.from({ length: count }, (_, i) => min + i * step);
	}

	let allValues = $derived(series.flatMap((s) => s.values.filter((v): v is number => v !== null)));

	let yTicks = $derived.by(() => {
		if (allValues.length === 0) return [];
		const min = Math.min(...allValues);
		const max = Math.max(...allValues);
		const ticks = niceTicks(min, max, 5);
		// A narrow value range can produce evenly-spaced ticks that all round to
		// the same displayed label (five ticks, all showing "125") — drop the
		// ones that would repeat the previous label rather than the caller
		// having to know its own formatter's rounding to avoid it.
		const deduped: number[] = [];
		let lastLabel: string | null = null;
		for (const tick of ticks) {
			const label = yFormat(tick);
			if (label === lastLabel) continue;
			deduped.push(tick);
			lastLabel = label;
		}
		return deduped;
	});

	// The y-axis gutter has to fit the widest label the formatter produces, not
	// a fixed guess: "24.7 km/h" needs twice the room of "41", and a constant
	// padLeft simply clipped it to ".7 km/h". Estimated from the character
	// count (~6.2px per glyph at 11px) rather than measured, which would mean
	// a DOM round-trip for a value that only needs to be roughly right.
	let padLeft = $derived.by(() => {
		if (bare) return 0;
		const widest = yTicks.reduce((max, tick) => Math.max(max, yFormat(tick).length), 0);
		return Math.max(narrow ? 34 : 38, Math.ceil(widest * 6.2) + 12);
	});

	// Driven by the raw data, not by yTicks: a narrow value range whose ticks
	// all round to the same displayed label used to collapse yTicks to one
	// element, and since yMin/yMax used to read off yTicks[0]/yTicks.at(-1),
	// the real max got silently discarded — every point then plotted at the
	// same height, a flat line pinned to the axis instead of the real
	// (small) variation. A perfectly flat series is padded so it draws
	// mid-height instead of on the edge.
	let yMin = $derived.by(() => {
		if (allValues.length === 0) return 0;
		const min = Math.min(...allValues);
		const max = Math.max(...allValues);
		return min === max ? min - Math.max(Math.abs(min) * 0.1, 0.5) : min;
	});
	let yMax = $derived.by(() => {
		if (allValues.length === 0) return 1;
		const min = Math.min(...allValues);
		const max = Math.max(...allValues);
		return min === max ? max + Math.max(Math.abs(max) * 0.1, 0.5) : max;
	});

	let xMin = $derived(xValues.length > 0 ? Math.min(...xValues) : 0);
	let xMax = $derived(xValues.length > 0 ? Math.max(...xValues) : 1);

	function scaleX(x: number): number {
		if (xMax === xMin) return padLeft;
		return padLeft + ((x - xMin) / (xMax - xMin)) * (width - padLeft - padRight);
	}

	function scaleY(y: number): number {
		if (yMax === yMin) return height - padBottom;
		return height - padBottom - ((y - yMin) / (yMax - yMin)) * (height - padTop - padBottom);
	}

	let xTicks = $derived(niceTicks(xMin, xMax, Math.min(narrow ? 3 : 5, xValues.length || 1)));

	let lines = $derived(
		series.map((s, seriesIndex) => {
			const pts = xValues
				.map((x, i) => (s.values[i] === null ? null : { x: scaleX(x), y: scaleY(s.values[i]!) }))
				.filter((p): p is { x: number; y: number } => p !== null);
			const areaBase = height - padBottom;
			const area =
				pts.length > 1
					? `M ${pts[0].x},${areaBase} ` +
						pts.map((p) => `L ${p.x},${p.y}`).join(' ') +
						` L ${pts[pts.length - 1].x},${areaBase} Z`
					: '';
			return {
				...s,
				gradientId: `chart-area-${instanceId}-${seriesIndex}`,
				polyline: pts.map((p) => `${p.x},${p.y}`).join(' '),
				area
			};
		})
	);

	let hoverIndex: number | null = $state(null);

	// Pointer rather than mouse events: the same handler then serves touch and
	// pen, so the tooltip works on the phone this app is mostly used on. The
	// stylesheet sets touch-action: pan-y so a vertical swipe still scrolls the
	// page while a horizontal drag scrubs the chart.
	function onMove(e: PointerEvent & { currentTarget: SVGSVGElement }) {
		if (xValues.length === 0) return;
		const rect = e.currentTarget.getBoundingClientRect();
		const relX = ((e.clientX - rect.left) / rect.width) * width;
		let closest = 0;
		let closestDist = Infinity;
		for (let i = 0; i < xValues.length; i++) {
			const dist = Math.abs(scaleX(xValues[i]) - relX);
			if (dist < closestDist) {
				closestDist = dist;
				closest = i;
			}
		}
		hoverIndex = closest;
	}

	function onLeave() {
		hoverIndex = null;
	}

	let hoverX = $derived(hoverIndex !== null ? scaleX(xValues[hoverIndex]) : null);
</script>

<div class="chart-card" class:bare>
	{#if !bare && series.length > 1}
		<ul class="legend">
			{#each series as s (s.name)}
				<li title={s.description}>
					<span class="swatch" style="background: {s.color}"></span>{s.name}
				</li>
			{/each}
		</ul>
	{/if}
	<!-- Measured on a padding-free wrapper: clientWidth on the card would
	     include its padding and reintroduce a small scale factor. -->
	<div class="plot" bind:clientWidth={measuredWidth}>
		<svg
			viewBox="0 0 {width} {height}"
			role="img"
			aria-label={ariaLabel}
			onpointermove={onMove}
			onpointerleave={onLeave}
			onpointercancel={onLeave}
		>
			<defs>
				{#each lines as line (line.name)}
					<linearGradient id={line.gradientId} x1="0" y1="0" x2="0" y2="1">
						<stop offset="0%" style="stop-color: {line.color}; stop-opacity: 0.25" />
						<stop offset="100%" style="stop-color: {line.color}; stop-opacity: 0" />
					</linearGradient>
				{/each}
				<linearGradient id="chart-hover-band-{instanceId}" x1="0" y1="0" x2="1" y2="0">
					<stop offset="0%" style="stop-color: var(--color-text-muted); stop-opacity: 0" />
					<stop offset="50%" style="stop-color: var(--color-text-muted); stop-opacity: 0.12" />
					<stop offset="100%" style="stop-color: var(--color-text-muted); stop-opacity: 0" />
				</linearGradient>
			</defs>
			{#if !bare}
				{#each yTicks as tick (tick)}
					<text x={padLeft - 8} y={scaleY(tick)} text-anchor="end" dominant-baseline="middle"
						>{yFormat(tick)}</text
					>
				{/each}
				{#each xTicks as tick, i (tick)}
					<!-- The outermost ticks sit on the plot edges, so a centred label
				     runs off the SVG and gets clipped mid-word ("25 mii"). Anchor
				     them inwards instead. -->
					<text
						x={scaleX(tick)}
						y={height - 8}
						text-anchor={i === 0 ? 'start' : i === xTicks.length - 1 ? 'end' : 'middle'}
						>{xFormat(tick)}</text
					>
				{/each}
			{/if}
			{#if baseline !== undefined}
				<line
					x1={padLeft}
					x2={width - padRight}
					y1={scaleY(baseline)}
					y2={scaleY(baseline)}
					stroke="var(--color-border)"
					stroke-width="1"
					stroke-dasharray="2 4"
				/>
			{/if}
			{#if hoverX !== null}
				<rect
					x={hoverX - 14}
					y={padTop}
					width="28"
					height={height - padTop - padBottom}
					fill="url(#chart-hover-band-{instanceId})"
				/>
			{/if}
			<!-- Draws itself in on mount (Nocturne v2) — clip-path rather than the
			     usual stroke-dasharray/getTotalLength trick, since it animates the
			     area fill and the line together with one rule and no per-path JS
			     measurement. Purely CSS, so prefers-reduced-motion's blanket
			     transition-duration override is enough to disable it. -->
			<g class="chart-ink">
				{#each lines as line (line.name)}
					{#if line.area}
						<path d={line.area} fill="url(#{line.gradientId})" stroke="none" />
					{/if}
					<polyline
						points={line.polyline}
						fill="none"
						stroke={line.color}
						stroke-width="2"
						stroke-linejoin="round"
						stroke-linecap="round"
					/>
				{/each}
			</g>
			{#if hoverX !== null && hoverIndex !== null}
				{#each series as s (s.name)}
					{@const v = s.values[hoverIndex]}
					{#if v !== null}
						<circle cx={hoverX} cy={scaleY(v)} r="4" fill={s.color} />
					{/if}
				{/each}
			{/if}
		</svg>
	</div>
	{#if !bare && hoverIndex !== null}
		<div class="tooltip">
			<strong>{xFormat(xValues[hoverIndex])}</strong>
			{#each series as s (s.name)}
				{@const v = s.values[hoverIndex]}
				{#if v !== null}
					<div>
						<span class="swatch" style="background: {s.color}"></span>{s.name}: {yFormat(v)}
					</div>
				{/if}
			{/each}
		</div>
	{/if}
</div>

<style>
	.plot {
		width: 100%;
		/* Caps the CSS width at the same value the viewBox caps at, so the
		   scale factor stays exactly 1 on wide screens too — otherwise the
		   chart is stretched and the labels grow past their intended size. */
		max-width: 800px;
	}

	/* No background/ring/padding (Nocturne v2) — with .panel also going bare,
	   a chart sitting in its own bordered box on top of that was a card
	   inside a card. The chart is legible directly on the page; touch-action
	   and the tooltip's own glass background still separate it from content
	   scrolling underneath. */
	.chart-card {
		position: relative;
	}

	.chart-card.bare {
		line-height: 0;
	}

	/* transform rather than clip-path: percentages in clip-path's inset()
	   resolve against an ambiguous reference box on a bare SVG <g>, but a
	   unitless scaleX is exact everywhere and CSS transforms on SVG have
	   been reliable for years. transform-box: view-box anchors the 0%
	   origin to the chart's own viewBox rather than the page. */
	.chart-ink {
		transform-box: view-box;
		transform-origin: left;
		transition: transform var(--dur-slow) var(--ease-out-soft);
	}

	@starting-style {
		.chart-ink {
			transform: scaleX(0);
		}
	}

	svg {
		display: block;
		width: 100%;
		height: auto;
		cursor: crosshair;
		/* Vertical swipes still scroll the page; horizontal drags scrub the
		   chart. Without this the browser claims the gesture and the tooltip
		   never fires on touch. */
		touch-action: pan-y;
	}

	text {
		font-size: 11px;
		fill: var(--color-text-muted);
	}

	.legend {
		display: flex;
		gap: 1rem;
		list-style: none;
		padding: 0;
		margin: 0 0 0.5rem;
		font-size: var(--text-sm);
	}

	.tooltip {
		position: absolute;
		top: 0.75rem;
		right: 0.75rem;
		background: var(--surface-glass);
		backdrop-filter: blur(12px);
		-webkit-backdrop-filter: blur(12px);
		border-radius: 12px;
		padding: 0.5rem 0.75rem;
		font-size: var(--text-sm);
		box-shadow: var(--shadow-lg);
		pointer-events: none;
	}

	.swatch {
		display: inline-block;
		width: 0.75rem;
		height: 0.75rem;
		border-radius: 50%;
		margin-right: 0.25rem;
	}
</style>
