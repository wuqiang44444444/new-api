export const MAX_CHANNEL_GROUP_LENGTH = 128

export const CHANNEL_GROUP_LENGTH_ERROR =
  'Channel groups must not exceed 128 characters (including commas)'

export function isChannelGroupLengthValid(groups: string[]): boolean {
  return [...groups.join(',')].length <= MAX_CHANNEL_GROUP_LENGTH
}
