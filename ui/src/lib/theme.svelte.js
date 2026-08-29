function createTheme() {
	let dark = $state(typeof document !== 'undefined' && document.documentElement.classList.contains('dark'));

	function apply() {
		document.documentElement.classList.toggle('dark', dark);
		try { localStorage.setItem('theme', dark ? 'dark' : 'light'); } catch {}
	}

	function toggle() {
		dark = !dark;
		apply();
	}

	return {
		get dark() { return dark; },
		toggle
	};
}

export const theme = createTheme();
