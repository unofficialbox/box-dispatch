import boxLogo from '../assets/providers/box.svg'
import bedrockLogo from '../assets/providers/bedrock.svg'
import salesforceLogo from '../assets/providers/salesforce.svg'

// Official full-color Databricks mark from brand.databricks.com.
const databricksLogo = 'data:image/png;base64,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'

type ProviderLogoProps = {
  provider: string
  size?: 'compact' | 'standard'
  decorative?: boolean
  className?: string
}

const providerAssets = {
  box: { label: 'Box', src: boxLogo },
  bedrock: { label: 'Amazon Bedrock', src: bedrockLogo },
  databricks: { label: 'Databricks', src: databricksLogo },
  salesforce: { label: 'Salesforce', src: salesforceLogo },
} as const

const providerKey = (provider: string) => {
  const key = provider.trim().toLowerCase()
  return key === 'amazon bedrock' || key === 'amazon-bedrock' ? 'bedrock' : key
}

function providerLabel(provider: string) {
  return providerAssets[providerKey(provider) as keyof typeof providerAssets]?.label || provider || 'Provider'
}

export function ProviderLogo({ provider, size = 'standard', decorative = true, className = '' }: ProviderLogoProps) {
  const key = providerKey(provider)
  const asset = providerAssets[key as keyof typeof providerAssets]
  const classes = ['provider-logo', `provider-logo--${size}`, className].filter(Boolean).join(' ')
  if (!asset) {
    const label = providerLabel(provider)
    return <span className={`${classes} provider-logo--fallback`} aria-hidden={decorative || undefined} aria-label={decorative ? undefined : `${label} logo`}>{label.slice(0, 2).toUpperCase()}</span>
  }
  return <span className={`${classes} provider-logo--${key}`}><img src={asset.src} alt={decorative ? '' : `${asset.label} logo`} data-provider-logo={key}/></span>
}

export function ProviderIdentity({ provider, size = 'compact' }: Pick<ProviderLogoProps, 'provider' | 'size'>) {
  return <span className="provider-identity"><ProviderLogo provider={provider} size={size}/><span>{providerLabel(provider)}</span></span>
}
