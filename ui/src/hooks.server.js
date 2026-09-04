import { dev } from '$app/environment';

const CSP = [
	"default-src 'self'",
	"script-src 'self'",
	"style-src 'self' 'unsafe-inline'",
	"img-src 'self' data: blob:",
	"connect-src 'self' https://tiles.openfreemap.org https://demotiles.maplibre.org",
	'worker-src blob:',
	"font-src 'self'",
	"frame-ancestors 'none'",
	"base-uri 'self'",
].join('; ');

/** @type {import('@sveltejs/kit').Handle} */
export async function handle({ event, resolve }) {
	const response = await resolve(event);
	if (!dev) {
		response.headers.set('Content-Security-Policy', CSP);
	}
	return response;
}
