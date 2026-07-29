<script lang="ts">
	// Reduced, geometric bike — no CDN icon set, no illustration library, just
	// a handful of SVG primitives. Deliberately plain: WFF's dark scheme has
	// twice been rejected as looking like industrial control-panel gear (see
	// memory wff-design-nicht-industriell), and a detailed/realistic bicycle
	// clipart risks the same read a third way.
	let { size = 160 }: { size?: number } = $props();
</script>

<svg
	viewBox="0 0 200 120"
	width={size}
	height={size * 0.6}
	class="bike"
	role="img"
	aria-label="Fahrrad-Piktogramm"
>
	<path class="frame" d="M40 90 L100 40 L140 90 M100 40 L84 90 M62 90 L140 90 L120 40" />
	<circle class="crank" cx="84" cy="90" r="6" />

	<g class="wheel wheel-rear">
		<circle class="rim" cx="40" cy="90" r="26" />
		<line class="spoke" x1="40" y1="64" x2="40" y2="116" />
		<line class="spoke" x1="14" y1="90" x2="66" y2="90" />
		<line class="spoke" x1="21.6" y1="71.6" x2="58.4" y2="108.4" />
		<line class="spoke" x1="21.6" y1="108.4" x2="58.4" y2="71.6" />
	</g>

	<g class="wheel wheel-front">
		<circle class="rim" cx="140" cy="90" r="26" />
		<line class="spoke" x1="140" y1="64" x2="140" y2="116" />
		<line class="spoke" x1="114" y1="90" x2="166" y2="90" />
		<line class="spoke" x1="121.6" y1="71.6" x2="158.4" y2="108.4" />
		<line class="spoke" x1="121.6" y1="108.4" x2="158.4" y2="71.6" />
	</g>
</svg>

<style>
	.bike {
		display: block;
		overflow: visible;
	}

	.frame {
		fill: none;
		stroke: var(--color-text);
		stroke-width: 5;
		stroke-linecap: round;
		stroke-linejoin: round;
	}

	.crank {
		fill: var(--color-text);
	}

	.rim {
		fill: none;
		stroke: var(--color-brand);
		stroke-width: 4;
	}

	.spoke {
		stroke: var(--color-brand);
		stroke-width: 2;
		opacity: 0.6;
	}

	/* fill-box + center gives each wheel its own rotation axis without having
	   to hand-match a pixel transform-origin to its position in the viewBox. */
	.wheel {
		transform-box: fill-box;
		transform-origin: center;
		animation: spin 3s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.wheel {
			animation: none;
		}
	}
</style>
