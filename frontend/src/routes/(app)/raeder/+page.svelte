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
	import {
		listActivities,
		bulkAssignBike,
		formatDistance,
		formatDuration,
		type ActivitySummary
	} from '$lib/rides';
	import { ApiError } from '$lib/api';

	let viewState: 'loading' | 'error' | 'ready' = $state('loading');
	let errorMessage = $state('');
	let bikeList: Bike[] = $state([]);
	let unassigned: ActivitySummary[] = $state([]);
	let newName = $state('');
	let busy = $state(false);

	// Backfill for rides uploaded before a bike existed, or without an active
	// bike set (#637/#729) — one at a time would be needless friction for
	// whatever a rider's history has accumulated.
	let selectedIds: Set<number> = $state(new Set());
	let assignBikeId: number | '' = $state('');
	let assignBusy = $state(false);
	let assignError = $state('');

	onMount(load);

	async function load() {
		try {
			const [bikes, activities] = await Promise.all([listBikes(), listActivities()]);
			bikeList = bikes;
			unassigned = activities.filter((a) => a.bike_id === null);
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

	function toggleSelected(id: number) {
		const next = new Set(selectedIds);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		selectedIds = next;
	}

	function toggleSelectAll() {
		selectedIds = selectedIds.size === unassigned.length ? new Set() : new Set(unassigned.map((a) => a.id));
	}

	function formatRideDate(a: ActivitySummary): string {
		return new Date(a.started_at).toLocaleDateString('de-DE', {
			day: '2-digit',
			month: '2-digit',
			year: 'numeric'
		});
	}

	async function assignSelected() {
		if (selectedIds.size === 0 || assignBikeId === '') return;
		assignBusy = true;
		assignError = '';
		try {
			await bulkAssignBike([...selectedIds], Number(assignBikeId));
			const assigned = selectedIds;
			unassigned = unassigned.filter((a) => !assigned.has(a.id));
			selectedIds = new Set();
			// Distance/ride counts just moved between bikes — refresh the
			// comparison figures rather than let them go stale until reload.
			bikeList = await listBikes();
		} catch (err) {
			assignError = err instanceof ApiError ? err.message : 'Zuweisung ist fehlgeschlagen.';
		} finally {
			assignBusy = false;
		}
	}
</script>

<div class="page-center">
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
				<li class="bike-row" class:retired={bike.retired_at}>
					<div class="bike-header">
						<strong>{bike.name}</strong>
						{#if bike.active}
							<span class="chip" style="--chip-color: var(--color-success)">Aktiv</span>
						{:else if bike.retired_at}
							<span class="chip" style="--chip-color: var(--color-text-muted)">Stillgelegt</span>
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
				<input
					id="new-bike-name"
					class="input"
					type="text"
					bind:value={newName}
					placeholder="Name"
				/>
				<button class="btn btn-primary" type="submit" disabled={busy || !newName.trim()}>
					Anlegen
				</button>
			</div>
		</form>

		<!-- Comparison (#731): honest cumulative figures, same as Strava/Garmin
		     Gear, deliberately not a constructed "which bike is faster" score —
		     speed depends too much on route and wind to attribute to the bike. -->
		{#if bikeList.length > 0}
			<section class="section">
				<h2>Vergleich</h2>
				{#if bikeList.length < 2}
					<p class="hint">Lege ein zweites Rad an, um sie hier zu vergleichen.</p>
				{:else}
					<div class="table-scroll">
						<table class="compare">
							<thead>
								<tr>
									<th>Rad</th>
									<th>Fahrten</th>
									<th>Distanz</th>
									<th>Zeit</th>
									<th>Höhenmeter</th>
									<th>⌀ Tempo</th>
								</tr>
							</thead>
							<tbody>
								{#each bikeList as bike (bike.id)}
									<tr class:retired={bike.retired_at}>
										<td
											>{bike.name}{#if bike.retired_at}<span class="muted"
													> (stillgelegt)</span
												>{/if}</td
										>
										<td>{bike.ride_count}</td>
										<td>{bike.distance_km.toFixed(0)} km</td>
										<td>{formatDuration(bike.moving_seconds)}</td>
										<td>{Math.round(bike.elevation_gain_meters)} m</td>
										<td
											>{bike.avg_speed_kmh > 0 ? `${bike.avg_speed_kmh.toFixed(1)} km/h` : '–'}</td
										>
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				{/if}
			</section>
		{/if}

		<!-- Backfill for the initial rollout (#729): rides from before this bike
		     existed, or uploaded without an active bike set. -->
		{#if unassigned.length > 0}
			<section class="section">
				<h2>Fahrten ohne Rad</h2>
				<p class="hint">
					{unassigned.length}
					{unassigned.length === 1 ? 'Fahrt wartet' : 'Fahrten warten'} noch auf ein Rad.
				</p>

				<ul class="unassigned">
					<li class="unassigned-row unassigned-all">
						<label>
							<input
								type="checkbox"
								checked={selectedIds.size === unassigned.length}
								onchange={toggleSelectAll}
							/>
							Alle auswählen
						</label>
					</li>
					{#each unassigned as activity (activity.id)}
						<li class="unassigned-row">
							<label>
								<input
									type="checkbox"
									checked={selectedIds.has(activity.id)}
									onchange={() => toggleSelected(activity.id)}
								/>
								{formatRideDate(activity)} · {formatDistance(activity.distance_meters)}
							</label>
						</li>
					{/each}
				</ul>

				<div class="assign-row">
					<select class="input" bind:value={assignBikeId} disabled={assignBusy}>
						<option value="">Rad wählen…</option>
						{#each bikeList as bike (bike.id)}
							<option value={bike.id}>{bike.name}{bike.retired_at ? ' (stillgelegt)' : ''}</option>
						{/each}
					</select>
					<button
						class="btn btn-primary"
						disabled={assignBusy || selectedIds.size === 0 || assignBikeId === ''}
						onclick={assignSelected}
					>
						{selectedIds.size > 0 ? `${selectedIds.size} zuweisen` : 'Zuweisen'}
					</button>
				</div>
				{#if assignError}
					<p role="alert" class="error-text">{assignError}</p>
				{/if}
			</section>
		{/if}
	{/if}
</div>

<style>
	.hint {
		color: var(--color-text-muted);
		font-size: var(--text-sm);
		max-width: 48ch;
	}

	/* Flat, divider-based list rather than a stack of shadowed cards — with
	   three sections now sharing this page, that reads calmer and more
	   modern than repeating the same boxed-card pattern three times over. */
	.bikes {
		list-style: none;
		padding: 0;
		margin: 1.25rem 0 2rem;
	}

	.bike-row {
		padding: 0.9rem 0;
		border-bottom: 1px solid var(--color-border);
	}

	.bike-row.retired {
		opacity: 0.7;
	}

	.bike-header {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 0.375rem;
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

	.section {
		margin-top: 2.5rem;
		padding-top: 1.5rem;
		border-top: 1px solid var(--color-border);
	}

	.section h2 {
		margin: 0 0 0.5rem;
	}

	.table-scroll {
		overflow-x: auto;
		margin-top: 1rem;
	}

	.compare {
		width: 100%;
		border-collapse: collapse;
		font-size: var(--text-sm);
		white-space: nowrap;
	}

	.compare th {
		text-align: left;
		color: var(--color-text-muted);
		font-weight: 700;
		padding: 0.5rem 0.75rem 0.5rem 0;
	}

	.compare td {
		padding: 0.6rem 0.75rem 0.6rem 0;
		border-top: 1px solid var(--color-border);
		font-variant-numeric: tabular-nums;
	}

	.compare tr.retired {
		color: var(--color-text-muted);
	}

	.muted {
		color: var(--color-text-muted);
		font-size: var(--text-xs);
	}

	.unassigned {
		list-style: none;
		padding: 0;
		margin: 1rem 0;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.unassigned-row {
		padding: 0.4rem 0;
		border-bottom: 1px solid var(--color-border);
		font-size: var(--text-sm);
	}

	.unassigned-all {
		font-weight: 700;
		border-bottom: 1px solid var(--color-text-muted);
	}

	.unassigned-row label {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		cursor: pointer;
	}

	.assign-row {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		align-items: center;
	}

	.assign-row select {
		/* flex-basis:0% via the shorthand overrides the global .input's
		   width:100% for sizing purposes — same trick the share-link row on
		   the ride-detail page already relies on. */
		flex: 1;
		min-width: 12rem;
	}

	.error-text {
		color: var(--color-danger);
		font-size: var(--text-sm);
		margin-top: 0.5rem;
	}
</style>
