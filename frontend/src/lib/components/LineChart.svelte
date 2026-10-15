<script module lang="ts">
	export interface Series {
		name: string;
		values: (number | null)[];
		// Shown as a native tooltip on the legend entry — cheapest way to
		// explain jargon (CTL/ATL/TSB) without a custom tooltip component.
		description?: string;
	}

	/** Round tick values ("nice numbers": 1, 2, 2.5 or 5 × 10ⁿ apart) inside
	 *  [min, max], about `target` of them, so a hairline sits on 20 and 40, not
	 *  on 23.7 and 41.3. */
	function roundTicks(min: number, max: number, target = 4): number[] {
		if (min === max) return [min];
		const raw = (max - min) / target;
		const magnitude = 10 ** Math.floor(Math.log10(raw));
		const step = [1, 2, 2.5, 5, 10].map((m) => m * magnitude).find((s) => s >= raw) ?? raw;
		const ticks: number[] = [];
		for (let t = Math.ceil(min / step) * step; t <= max + step * 1e-9; t += step) {
			ticks.push(Number(t.toFixed(10)));
		}
		return ticks;
	}
</script>

<script lang="ts">
	// Shared line chart in Rekta's data rules (docs/data.md, ADR
	// adr-003-wff-datenfarben-rekta): ink and grey, never a series palette —
	// the first series in --rk-text-primary, a second in
	// --rk-text-contrast-medium; three or more become small multiples. One y
	// axis, hairline grid at round ticks, a crosshair and a tooltip with every
	// value under the pointer, ← → Home End along the points, and the numbers
	// as a table behind "Zahlen zeigen". Lines draw in once (rk-draw).
	import LineChart from './LineChart.svelte';

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
		// A reference line ("Frische über null ist erholt") at this y-value,
		// drawn a step darker than the grid so it reads as a reference, never as
		// another series.
		baseline?: number;
		// Only the line, for a small full-bleed strip (the ride-detail header's
		// elevation profile) where the shape alone is the point.
		bare?: boolean;
	} = $props();

	// The viewBox tracks the measured width so one SVG unit is one CSS pixel
	// (#605): an axis label stays its real size on a phone.
	let measuredWidth = $state(0);
	let width = $derived(Math.min(measuredWidth || maxWidth, maxWidth));

	// A phone can't fit five date labels without them colliding.
	let narrow = $derived(width < 480);
	let padRight = $derived(bare ? 1 : 12);
	let padTop = $derived(bare ? 2 : 12);
	let padBottom = $derived(bare ? 2 : narrow ? 24 : 28);

	let allValues = $derived(series.flatMap((s) => s.values.filter((v): v is number => v !== null)));
	let dataMin = $derived(allValues.length ? Math.min(...allValues) : 0);
	let dataMax = $derived(allValues.length ? Math.max(...allValues) : 1);

	// Lines may start where the data does (Rekta); a perfectly flat series is
	// padded so it draws mid-height instead of on the edge.
	let yMin = $derived(
		dataMin === dataMax ? dataMin - Math.max(Math.abs(dataMin) * 0.1, 0.5) : dataMin
	);
	let yMax = $derived(
		dataMin === dataMax ? dataMax + Math.max(Math.abs(dataMax) * 0.1, 0.5) : dataMax
	);

	let yTicks = $derived.by(() => {
		if (allValues.length === 0) return [];
		// A tick whose label repeats the previous one (the formatter rounds) says
		// nothing new.
		const deduped: number[] = [];
		let last: string | null = null;
		for (const tick of roundTicks(yMin, yMax)) {
			const label = yFormat(tick);
			if (label === last) continue;
			deduped.push(tick);
			last = label;
		}
		return deduped;
	});

	// The y-axis gutter fits the widest label, estimated at ~7.4px per glyph at
	// Rekta's chart size (font-size-xs, 13px), not measured: roughly right is
	// enough and saves a DOM round-trip.
	let padLeft = $derived.by(() => {
		if (bare) return 1;
		const widest = yTicks.reduce((max, tick) => Math.max(max, yFormat(tick).length), 0);
		return Math.max(narrow ? 36 : 40, Math.ceil(widest * 7.4) + 10);
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

	let xTicks = $derived.by(() => {
		const count = Math.min(narrow ? 3 : 5, xValues.length || 1);
		if (count === 1 || xMin === xMax) return [xMin];
		return Array.from({ length: count }, (_, i) => xMin + (i * (xMax - xMin)) / (count - 1));
	});

	let lines = $derived(
		series.map((s) => ({
			...s,
			points: xValues
				.map((x, i) => (s.values[i] === null ? null : `${scaleX(x)},${scaleY(s.values[i]!)}`))
				.filter((p) => p !== null)
				.join(' ')
		}))
	);

	let hoverIndex: number | null = $state(null);
	let hoverX = $derived(hoverIndex !== null ? scaleX(xValues[hoverIndex]) : null);

	function valuesAt(i: number): string {
		return series
			.filter((s) => s.values[i] !== null)
			.map((s) => `${s.name} ${yFormat(s.values[i]!)}`)
			.join(', ');
	}

	// Pointer events serve mouse, touch and pen alike; touch-action: pan-y
	// (Rekta's .rk-chart-svg) keeps vertical swipes scrolling the page.
	function onMove(e: PointerEvent & { currentTarget: SVGSVGElement }) {
		if (xValues.length === 0) return;
		const rect = e.currentTarget.getBoundingClientRect();
		const relX = ((e.clientX - rect.left) / rect.width) * width;
		let closest = 0;
		for (let i = 1; i < xValues.length; i++) {
			if (Math.abs(scaleX(xValues[i]) - relX) < Math.abs(scaleX(xValues[closest]) - relX))
				closest = i;
		}
		hoverIndex = closest;
	}

	function onKey(e: KeyboardEvent) {
		const last = xValues.length - 1;
		const at = hoverIndex ?? -1;
		const next =
			e.key === 'ArrowRight'
				? Math.min(at + 1, last)
				: e.key === 'ArrowLeft'
					? Math.max(at - 1, 0)
					: e.key === 'Home'
						? 0
						: e.key === 'End'
							? last
							: null;
		if (next === null || last < 0) return;
		e.preventDefault();
		hoverIndex = next;
	}

	// The table shows at most ~60 rows: a ride has a sample a second, and
	// thousands of rows help no one. Built only while open.
	let tableOpen = $state(false);
	let tableRows = $derived.by(() => {
		if (!tableOpen) return [];
		const step = Math.max(1, Math.ceil(xValues.length / 60));
		return xValues
			.map((x, i) => ({ x, i }))
			.filter(({ i }) => i % step === 0 || i === xValues.length - 1);
	});
</script>

{#if series.length > 2 && !bare}
	<!-- Three or more series are small multiples, never a third grey (Rekta). -->
	<div class="multiples">
		{#each series as s (s.name)}
			<div>
				<p class="rk-kicker" title={s.description}>{s.name}</p>
				<LineChart
					{xValues}
					series={[s]}
					{xFormat}
					{yFormat}
					ariaLabel="{ariaLabel}: {s.name}"
					{maxWidth}
					height={Math.max(96, Math.round(height / 2))}
					{baseline}
				/>
			</div>
		{/each}
	</div>
{:else}
	<div class="rk-chart" class:bare>
		{#if !bare && series.length > 1}
			<ul class="rk-legend">
				{#each series as s, i (s.name)}
					<li title={s.description}><i class="rk-key-line rk-s{i}"></i>{s.name}</li>
				{/each}
			</ul>
		{/if}
		<div class="plot" bind:clientWidth={measuredWidth}>
			<!-- An image you can step through with ← → Home End, as Rekta charts are; the
			     values are read out by the status line below. -->
			<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
			<svg
				class="rk-chart-svg"
				viewBox="0 0 {width} {height}"
				role="img"
				aria-label={ariaLabel}
				tabindex={bare ? undefined : 0}
				onpointermove={bare ? undefined : onMove}
				onpointerleave={() => (hoverIndex = null)}
				onpointercancel={() => (hoverIndex = null)}
				onkeydown={bare ? undefined : onKey}
				onblur={() => (hoverIndex = null)}
			>
				{#if !bare}
					<g class="rk-chart-grid">
						{#each yTicks as tick (tick)}
							<line x1={padLeft} x2={width - padRight} y1={scaleY(tick)} y2={scaleY(tick)} />
							<text x={padLeft - 8} y={scaleY(tick)} text-anchor="end" dominant-baseline="middle"
								>{yFormat(tick)}</text
							>
						{/each}
						{#each xTicks as tick, i (tick)}
							<!-- The outer labels sit on the plot edges; anchored inwards so
							     they are not clipped mid-word. -->
							<text
								x={scaleX(tick)}
								y={height - 8}
								text-anchor={i === 0 ? 'start' : i === xTicks.length - 1 ? 'end' : 'middle'}
								>{xFormat(tick)}</text
							>
						{/each}
					</g>
				{/if}
				{#if baseline !== undefined && baseline >= yMin && baseline <= yMax}
					<line
						class="rk-chart-cross"
						x1={padLeft}
						x2={width - padRight}
						y1={scaleY(baseline)}
						y2={scaleY(baseline)}
					/>
				{/if}
				{#each lines as line, i (line.name)}
					<polyline class="rk-line rk-s{i}" points={line.points} pathLength="1" data-rk-draw />
				{/each}
				{#if hoverX !== null && hoverIndex !== null}
					<line
						class="rk-chart-cross"
						x1={hoverX}
						x2={hoverX}
						y1={padTop}
						y2={height - padBottom}
					/>
					{#each series as s, i (s.name)}
						{@const v = s.values[hoverIndex]}
						{#if v !== null}
							<circle class="rk-chart-dot rk-s{i}" cx={hoverX} cy={scaleY(v)} r="4" />
						{/if}
					{/each}
				{/if}
			</svg>
		</div>
		{#if !bare && hoverIndex !== null}
			<div class="tip" aria-hidden="true">
				<strong>{xFormat(xValues[hoverIndex])}</strong>
				{#each series as s (s.name)}
					{@const v = s.values[hoverIndex]}
					{#if v !== null}
						<span>{series.length > 1 ? `${s.name}: ` : ''}{yFormat(v)}</span>
					{/if}
				{/each}
			</div>
		{/if}
		{#if !bare}
			<p class="rk-sr" role="status">
				{hoverIndex !== null ? `${xFormat(xValues[hoverIndex])}: ${valuesAt(hoverIndex)}` : ''}
			</p>
			<details class="rk-chart-table" bind:open={tableOpen}>
				<summary>Zahlen zeigen</summary>
				{#if tableOpen}
					<table class="rk-table">
						<thead>
							<tr>
								<th scope="col"></th>
								{#each series as s (s.name)}<th scope="col">{s.name}</th>{/each}
							</tr>
						</thead>
						<tbody>
							{#each tableRows as row (row.i)}
								<tr>
									<th scope="row">{xFormat(row.x)}</th>
									{#each series as s (s.name)}
										<td>{s.values[row.i] === null ? '–' : yFormat(s.values[row.i]!)}</td>
									{/each}
								</tr>
							{/each}
						</tbody>
					</table>
				{/if}
			</details>
		{/if}
	</div>
{/if}

<style>
	.plot {
		width: 100%;
		/* Caps the CSS width at the viewBox cap, so one unit stays one pixel on
		   wide screens too. */
		max-width: 800px;
	}

	.rk-chart-svg {
		width: 100%;
	}

	.bare {
		line-height: 0;
	}

	.bare .rk-chart-svg {
		pointer-events: none;
	}

	.multiples {
		display: grid;
		gap: var(--rk-space-l);
	}

	/* A small ink strip with every value at the crosshair, like Rekta's
	   tooltip; it stays in the corner so it never covers the point read. */
	.tip {
		position: absolute;
		top: 0;
		right: 0;
		display: grid;
		padding: var(--rk-space-xs) var(--rk-space-s);
		border-radius: var(--rk-radius-s);
		background: var(--rk-text-primary);
		color: var(--rk-bg);
		font-size: var(--rk-font-size-xs);
		font-variant-numeric: tabular-nums;
		pointer-events: none;
	}

	.rk-table th,
	.rk-table td {
		text-align: left;
		padding-right: var(--rk-space-m);
	}
</style>
