import { readFileSync } from 'node:fs'
import type { SeedancePluginConfiguration } from '../seedance-plugin-configuration'

const source = readFileSync('../plugins/seedance-link/plugin.js', 'utf8')
// Execute the repository-owned pure artifact, without Provider I/O or keys.
export const publishedSeedanceConfiguration = new Function(`${source.replaceAll('export ', '')  }; return meta.channelConfiguration`)() as SeedancePluginConfiguration
