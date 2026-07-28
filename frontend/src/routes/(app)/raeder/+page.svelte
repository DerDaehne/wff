<script lang="ts">
	import { onMount } from 'svelte';
	import {
		listBikes,
		createBike,
		updateBike,
		activateBike,
		markChainReplaced,
		type Bike
	} from '$lib/bikes';
	import { ApiError } from '$lib/api';

	let viewState: 'loading' | 'error' | 'ready' = $state('loading');
	let errorMessage = $state('');
	let bikeList: Bike[] = $state([]);
	let newName = $state('');
	let busy = $state(false);

	onMount(load);

	async function load() {
		try {
			bikeList = await listBikes();
			viewState = 'ready';
		} catch (err) {
			errorMessage = err instanceof ApiError ? err.message : 'Räder konnten nicht geladen werden.';
			viewState = 'error';
		}
	}

	async function run(action: () => Promise<Bike[]>) {
		busy = true;
		try {
			bikeList = await action();
		} catch (err) {
			errorMessage = err instanceof ApiError ? err.message : 'Aktion ist fehlgeschlagen.';
			viewState = 'error';
		} finally {
			busy = false;
		}
	}

	async function addBike(e: SubmitEvent) {
		e.preventDefault();
		if (!newName.trim()) return;
		await run(() => createBike(newName.trim()));
		newName = '';
	}

	function chainLabel(bike: Bike): string {
		if (bike.chain_due_km <= 0) {
			return `Kette überfällig — ${Math.round(-bike.chain_due_km)} km über dem Intervall`;
		}
		return `Noch ${Math.round(bike.chain_due_km)} km bis zum Kettenwechsel`;
	}
</script>

<h1>Deine Räder</h1>

{#if viewState === 'loading'}
	<p>Lädt…</p>
{:else if viewState === 'error'}
	<p role="alert">{errorMessage}</p>
{:else}
	<p class="hint">
		Neue Fahrten werden automatisch dem aktiven Rad zugeordnet — beim Hochladen selbst musst du
		nichts auswählen.
	</p>

	<ul class="bikes">
		{#each bikeList as bike (bike.id)}
			<li class="bike" class:retired={bike.retired_at}>
				<div class="bike-header">
					<strong>{bike.name}</strong>
					{#if bike.active}
						<span class="badge">Aktiv</span>
					{:else if bike.retired_at}
						<span class="badge badge-muted">Stillgelegt</span>
					{/if}
				</div>
				<p class="stat">{bike.distance_km.toFixed(0)} km gefahren</p>
				<p class="stat" class:overdue={bike.chain_due_km <= 0}>{chainLabel(bike)}</p>
				<div class="actions">
					{#if !bike.active && !bike.retired_at}
						<button
							class="btn btn-secondary"
							disabled={busy}
							onclick={() => run(() => activateBike(bike.id))}
						>
							Als aktiv setzen
						</button>
					{/if}
					<button
						class="btn btn-secondary"
						disabled={busy}
						onclick={() => run(() => markChainReplaced(bike.id))}
					>
						Kette gewechselt
					</button>
					{#if bike.retired_at}
						<button
							class="btn btn-secondary"
							disabled={busy}
							onclick={() => run(() => updateBike(bike.id, { retired: false }))}
						>
							Reaktivieren
						</button>
					{:else}
						<button
							class="btn btn-secondary"
							disabled={busy}
							onclick={() => run(() => updateBike(bike.id, { retired: true }))}
						>
							Stilllegen
						</button>
					{/if}
				</div>
			</li>
		{/each}
	</ul>

	<form class="add-bike" onsubmit={addBike}>
		<label for="new-bike-name">Neues Rad</label>
		<div class="add-bike-row">
			<input id="new-bike-name" class="input" type="text" bind:value={newName} placeholder="Name" />
			<button class="btn btn-primary" type="submit" disabled={busy || !newName.trim()}>
				Anlegen
			</button>
		</div>
	</form>
{/if}

<style>
	.hint {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
		max-width: 40ch;
	}

	.bikes {
		list-style: none;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		margin: 1.5rem 0;
	}

	.bike {
		background: var(--color-surface);
		border-radius: var(--radius-md);
		box-shadow: var(--shadow-sm);
		padding: 1.25rem;
	}

	.bike.retired {
		opacity: 0.7;
	}

	.bike-header {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 0.5rem;
	}

	.badge {
		font-size: var(--text-xs);
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--color-success);
	}

	.badge-muted {
		color: var(--color-text-muted);
	}

	.stat {
		margin: 0.25rem 0;
		font-variant-numeric: tabular-nums;
	}

	.stat.overdue {
		color: var(--color-danger);
		font-weight: 700;
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		margin-top: 0.75rem;
	}

	.add-bike {
		max-width: 24rem;
	}

	.add-bike-row {
		display: flex;
		gap: 0.5rem;
	}

	.add-bike-row .input {
		flex: 1;
	}
</style>
