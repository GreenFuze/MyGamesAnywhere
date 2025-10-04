/**
 * LaunchBox Platform Types
 */

/**
 * LaunchBox metadata record from database
 */
export interface LaunchBoxMetadata {
  id: string;
  name: string;
  platform: string;
  developer?: string;
  publisher?: string;
  releaseDate?: string;
  overview?: string;
  genres?: string[];
  rating?: number;
}
