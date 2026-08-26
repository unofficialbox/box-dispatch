const defaultScratchOrgName = 'Box Dispatch'

export function scratchOrgRequest(alias: string, deploymentName: string, installManagedPackage = false) {
  return {
    alias: alias.trim(),
    orgName: deploymentName.trim() || defaultScratchOrgName,
    durationDays: 30,
    installManagedPackage,
  }
}
