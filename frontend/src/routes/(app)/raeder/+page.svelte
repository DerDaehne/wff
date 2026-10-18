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
		formatDecimal,
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
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- local copy, assigned back whole
		const next = new Set(selectedIds);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		selectedIds = next;
	}

	function toggleSelectAll() {
		selectedIds =
			selectedIds.size === unassigned.length ? new Set() : new Set(unassigned.map((a) => a.id));
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

<h1 class="rk-title">Deine Räder</h1>

{#if viewState === 'loading'}
	<p>Lädt…</p>
{:else if viewState === 'error'}
	<p role="alert">{errorMessage}</p>
{:else}
	<p class="sub">
		Neue Fahrten werden automatisch dem aktiven Rad zugeordnet — beim Hochladen selbst musst du
		nichts auswählen.
	</p>

	<!-- Rekta rows: the bike's state as a tag (a square and a word), its
	     actions as buttons with a verb. The active bike is "active" in Rekta's
	     sense (info); a chain past its interval waits for you (warning). -->
	<ul class="rk-rows bikes">
		{#each bikeList as bike (bike.id)}
			<li>
				<div class="bike">
					<p class="name">
						<b>{bike.name}</b>
						{#if bike.active}
							<span class="rk-tag" data-status="info">aktiv</span>
						{:else if bike.retired_at}
							<span class="rk-tag">stillgelegt</span>
						{/if}
						{#if bike.chain_due_km <= 0 && !bike.retired_at}
							<span class="rk-tag" data-status="warning">Kette fällig</span>
						{/if}
					</p>
					<p class="rk-meta">{bike.distance_km.toFixed(0)} km gefahren · {chainLabel(bike)}</p>
				</div>
				<p class="rk-actions">
					{#if !bike.active && !bike.retired_at}
						<button class="rk-btn" disabled={busy} onclick={() => run(() => activateBike(bike.id))}>
							Als aktiv setzen
						</button>
					{/if}
					<button
						class="rk-btn"
						disabled={busy}
						onclick={() => run(() => markChainReplaced(bike.id))}
					>
						Kette gewechselt
					</button>
					{#if bike.retired_at}
						<button
							class="rk-btn"
							disabled={busy}
							onclick={() => run(() => updateBike(bike.id, { retired: false }))}
						>
							Reaktivieren
						</button>
					{:else}
						<button
							class="rk-btn"
							disabled={busy}
							onclick={() => run(() => updateBike(bike.id, { retired: true }))}
						>
							Stilllegen
						</button>
					{/if}
				</p>
			</li>
		{/each}
	</ul>

	<form class="add" onsubmit={addBike}>
		<div class="rk-field">
			<label class="rk-label" for="new-bike-name">Neues Rad</label>
			<input
				id="new-bike-name"
				class="rk-input"
				type="text"
				bind:value={newName}
				placeholder="Name"
			/>
		</div>
		<button class="rk-btn rk-btn--primary" type="submit" disabled={busy || !newName.trim()}>
			Anlegen
		</button>
	</form>

	<!-- Comparison (#731): honest cumulative figures, same as Strava/Garmin
	     Gear, deliberately not a constructed "which bike is faster" score. -->
	{#if bikeList.length > 0}
		<section class="sec">
			<h2 class="rk-group">Vergleich</h2>
			{#if bikeList.length < 2}
				<p class="rk-empty">Lege ein zweites Rad an, um sie hier zu vergleichen.</p>
			{:else}
				<div class="rk-table-wrap">
					<table class="rk-table">
						<thead>
							<tr>
								<th scope="col">Rad</th>
								<th scope="col">Fahrten</th>
								<th scope="col">Distanz</th>
								<th scope="col">Zeit</th>
								<th scope="col">Höhenmeter</th>
								<th scope="col">⌀ Tempo</th>
							</tr>
						</thead>
						<tbody>
							{#each bikeList as bike (bike.id)}
								<tr>
									<th scope="row"
										>{bike.name}{#if bike.retired_at}&nbsp;<span class="rk-meta">(stillgelegt)</span
											>{/if}</th
									>
									<td data-label="Fahrten">{bike.ride_count}</td>
									<td data-label="Distanz">{bike.distance_km.toFixed(0)} km</td>
									<td data-label="Zeit">{formatDuration(bike.moving_seconds)}</td>
									<td data-label="Höhenmeter">{Math.round(bike.elevation_gain_meters)} m</td>
									<td data-label="⌀ Tempo"
										>{bike.avg_speed_kmh > 0
											? `${formatDecimal(bike.avg_speed_kmh, 1)} km/h`
											: '–'}</td
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
		<section class="sec">
			<h2 class="rk-group">Fahrten ohne Rad</h2>
			<p class="sub">
				{unassigned.length}
				{unassigned.length === 1 ? 'Fahrt wartet' : 'Fahrten warten'} noch auf ein Rad.
			</p>

			<ul class="rk-rows">
				<li>
					<label class="rk-switch">
						<input
							type="checkbox"
							checked={selectedIds.size === unassigned.length}
							onchange={toggleSelectAll}
						/>
						Alle auswählen
					</label>
				</li>
				{#each unassigned as activity (activity.id)}
					<li>
						<label class="rk-switch">
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

			<div class="add">
				<div class="rk-field" data-status={assignError ? 'error' : undefined}>
					<label class="rk-label" for="assign-bike">Zuweisen an</label>
					<select id="assign-bike" class="rk-input" bind:value={assignBikeId} disabled={assignBusy}>
						<option value="">Rad wählen…</option>
						{#each bikeList as bike (bike.id)}
							<option value={bike.id}>{bike.name}{bike.retired_at ? ' (stillgelegt)' : ''}</option>
						{/each}
					</select>
					{#if assignError}<p class="rk-hint" role="alert">{assignError}</p>{/if}
				</div>
				<button
					class="rk-btn rk-btn--primary"
					disabled={assignBusy || selectedIds.size === 0 || assignBikeId === ''}
					onclick={assignSelected}
				>
					{selectedIds.size > 0 ? `${selectedIds.size} zuweisen` : 'Zuweisen'}
				</button>
			</div>
		</section>
	{/if}
{/if}

<style>
	h1 {
		margin-bottom: var(--rk-space-s);
	}

	.sub {
		max-width: 60ch;
		margin-bottom: var(--rk-space-m);
		color: var(--rk-text-contrast-high);
	}

	.bikes > li {
		align-items: center;
	}

	.name {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--rk-space-s);
	}

	/* A field and its button on one line, the button on the field's baseline. */
	.add {
		display: flex;
		flex-wrap: wrap;
		align-items: end;
		gap: var(--rk-space-s);
		max-width: 36rem;
		margin-top: var(--rk-space-l);
	}

	.add .rk-field {
		flex: 1 1 14rem;
	}

	.sec {
		margin-top: var(--rk-space-xl);
	}
</style>
