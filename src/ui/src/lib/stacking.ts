/**
 * Z-order for portaled UI (Radix Portal → document.body).
 *
 * - Dialog overlay/content: z-[10050] / z-[10051] (see `components/ui/dialog.tsx`).
 * - FloatingWindow while dragging/resizing: up to ~60_000 + `topZ`
 *   (see `components/ui/floating-window.tsx`).
 *
 * Portaled selects/menus/tooltips must stay above all of the above during normal use.
 */
export const Z_PORTAL_POPOVER = 100_060
