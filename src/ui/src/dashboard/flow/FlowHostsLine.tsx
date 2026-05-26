import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import type { Host } from './flow-data'
import { shortcutIPv6 } from './flow-data'

interface FlowHostsLineProps {
  hosts: Host[]
}

function DefaultHostGlyph() {
  return (
    <svg
      className="defaultaliasimg"
      viewBox="0 0 24 24"
      fill="currentColor"
      aria-hidden="true"
    >
      <rect x="3" y="4" width="18" height="16" rx="3" opacity="0.15" />
      <circle cx="12" cy="10" r="3" />
      <path d="M6 18c1-3 4-4.5 6-4.5s5 1.5 6 4.5" opacity="0.8" />
    </svg>
  )
}

function HostHoverCard({ host }: { host: Host }) {
  const hasMultiplePorts = host.ports && host.ports.length > 1
  const portTooltip =
    hasMultiplePorts && host.ports?.length
      ? `Ports: ${host.ports.join(', ')}`
      : host.port
        ? `Port: ${host.port}`
        : ''
  const hasMeta =
    !!host.displayLabel || !!host.aliasImage || (host.aliasTags && host.aliasTags.length > 0)

  const ipTooltip =
    host.ips && host.ips.length > 1 ? `IPs: ${host.ips.join(', ')}` : ''

  return (
    <div className="flex max-w-[260px] flex-col gap-2 text-left font-normal">
      <div className="font-mono text-[11px] leading-snug opacity-95">{host.ip}</div>
      {ipTooltip ? <div className="text-[10px] opacity-80">{ipTooltip}</div> : null}
      {portTooltip ? <div className="text-[10px] opacity-80">{portTooltip}</div> : null}
      {hasMeta ? (
        <div className="space-y-2 border-t border-white/15 pt-2">
          {host.displayLabel ? (
            <div className="text-[11px] font-semibold leading-snug">{host.displayLabel}</div>
          ) : null}
          {host.aliasImage ? (
            <img
              src={host.aliasImage}
              alt=""
              className="mx-auto max-h-44 max-w-[220px] rounded object-contain"
            />
          ) : null}
          {host.aliasTags?.map((t, i) => (
            <div key={`${i}-${t}`} className="text-[10px] leading-snug">
              <span className="opacity-65">Tag {i + 1}</span>
              <span className="opacity-90"> · {t}</span>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  )
}

export function FlowHostsLine({ hosts }: FlowHostsLineProps) {
  return (
    <div className="hosts">
      <div className="hosts-wrapper">
        {hosts.map((host) => {
          const hasMultiplePorts = host.ports && host.ports.length > 1
          const portSummary = hasMultiplePorts
            ? `${host.ports!.length} ports`
            : host.port
              ? String(host.port)
              : ''
          const hasRichTip =
            !!host.displayLabel ||
            !!host.aliasImage ||
            (host.aliasTags && host.aliasTags.length > 0)

          return (
            <div key={host.key} className="item-wrapper">
              <Tooltip>
                <TooltipTrigger asChild>
                  <div
                    className={`item outline-none ${hasRichTip ? 'cursor-help' : 'cursor-default'}`}
                  >
                    <div className="aliasfield">
                      {host.aliasImage ? (
                        <img
                          src={host.aliasImage}
                          alt=""
                          className="mx-auto h-8 w-8 rounded border border-border/50 object-cover"
                          loading="lazy"
                        />
                      ) : (
                        <DefaultHostGlyph />
                      )}
                      <div className="alias-name">{host.displayLabel || shortcutIPv6(host.ip)}</div>
                    </div>
                    <div className="ipfield">{portSummary}</div>
                  </div>
                </TooltipTrigger>
                <TooltipContent
                  side="bottom"
                  sideOffset={6}
                  className="z-[10000] flex max-w-[280px] flex-col items-start gap-0 p-2.5 text-left shadow-lg"
                >
                  <HostHoverCard host={host} />
                </TooltipContent>
              </Tooltip>
            </div>
          )
        })}
      </div>
    </div>
  )
}
