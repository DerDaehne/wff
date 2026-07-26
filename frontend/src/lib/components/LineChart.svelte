<script lang="ts">
	// Shared chart primitive per design-wff-ui-konzept (#582/#145): every chart
	// gets labeled axes, gridlines, a legend when there's more than one series,
	// and a hover tooltip with the exact value — not just a bare polyline.
	interface Series {
		name: string;
		color: string;
		values: (number | null)[];
	}

	let {
		xValues,
		series,
		xFormat,
		yFormat,
		ariaLabel,
		width = 800,
		height = 220
	}: {
		xValues: number[];
		series: Series[];
		xFormat: (x: number) => string;
		yFormat: (y: number) => string;
		ariaLabel: string;
		width?: number;
		height?: number;
	} = $props();

	const padLeft = 48;
	const padRight = 12;
	const padTop = 12;
	const padBottom = 28;

	function niceTicks(min: number, max: number, count: number): number[] {
		if (min === max) return [min];
		const step = (max - min) / (count - 1);
		return Array.from({ length: count }, (_, i) => min + i * step);
	}

	let yTicks = $derived.by(() => {
		const allValues = series.flatMap((s) => s.values.filter((v): v is number => v !== null));
		if (allValues.length === 0) return [];
		const min = Math.min(...allValues);
		const max = Math.max(...allValues);
		return niceTicks(min, max, 5);
	});

	let yMin = $derived(yTicks.length > 0 ? yTicks[0] : 0);
	let yMax = $derived(yTicks.length > 0 ? yTicks[yTicks.length - 1] : 1);

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

	let xTicks = $derived(niceTicks(xMin, xMax, Math.min(5, xValues.length || 1)));

	let lines = $derived(
		series.map((s) => ({
			...s,
			points: xValues
				.map((x, i) => (s.values[i] === null ? null : { x: scaleX(x), y: scaleY(s.values[i]!) }))
				.filter((p): p is { x: number; y: number } => p !== null)
				.map((p) => `${p.x},${p.y}`)
				.join(' ')
		}))
	);

	let hoverIndex: number | null = $state(null);

	function onMove(e: MouseEvent & { currentTarget: SVGSVGElement }) {
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

<div class="chart-wrap">
	{#if series.length > 1}
		<ul class="legend">
			{#each series as s (s.name)}
				<li><span class="swatch" style="background: {s.color}"></span>{s.name}</li>
			{/each}
		</ul>
	{/if}
	<svg
		viewBox="0 0 {width} {height}"
		role="img"
		aria-label={ariaLabel}
		onmousemove={onMove}
		onmouseleave={onLeave}
	>
		{#each yTicks as tick (tick)}
			<line
				x1={padLeft}
				y1={scaleY(tick)}
				x2={width - padRight}
				y2={scaleY(tick)}
				stroke="var(--color-border, #e2e8f0)"
			/>
			<text x={padLeft - 6} y={scaleY(tick)} text-anchor="end" dominant-baseline="middle"
				>{yFormat(tick)}</text
			>
		{/each}
		{#each xTicks as tick (tick)}
			<text x={scaleX(tick)} y={height - 8} text-anchor="middle">{xFormat(tick)}</text>
		{/each}
		{#each lines as line (line.name)}
			<polyline
				points={line.points}
				fill="none"
				stroke={line.color}
				stroke-width="2"
				stroke-linejoin="round"
				stroke-linecap="round"
			/>
		{/each}
		{#if hoverX !== null && hoverIndex !== null}
			<line x1={hoverX} y1={padTop} x2={hoverX} y2={height - padBottom} class="hover-guide" />
			{#each series as s (s.name)}
				{@const v = s.values[hoverIndex]}
				{#if v !== null}
					<circle cx={hoverX} cy={scaleY(v)} r="3.5" fill={s.color} />
				{/if}
			{/each}
		{/if}
	</svg>
	{#if hoverIndex !== null}
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
	.chart-wrap {
		position: relative;
	}

	svg {
		width: 100%;
		height: auto;
		max-width: 800px;
		cursor: crosshair;
	}

	text {
		font-size: 11px;
		fill: var(--color-text-muted, #64748b);
	}

	.hover-guide {
		stroke: var(--color-text-muted, #64748b);
		stroke-width: 1;
		stroke-dasharray: 3 3;
	}

	.legend {
		display: flex;
		gap: 1rem;
		list-style: none;
		padding: 0;
		margin: 0 0 0.5rem;
		font-size: var(--text-sm, 0.875rem);
	}

	.tooltip {
		position: absolute;
		top: 0.5rem;
		right: 0.5rem;
		background: var(--color-surface, #fff);
		border: 1px solid var(--color-border, #e2e8f0);
		border-radius: 8px;
		padding: 0.5rem 0.75rem;
		font-size: var(--text-sm, 0.875rem);
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
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
