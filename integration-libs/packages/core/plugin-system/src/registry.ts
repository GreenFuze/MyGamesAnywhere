/**
 * Plugin Registry
 * Manages plugin registration and discovery
 */

import { PluginType } from './types.js';
import type {
  Plugin,
  SourcePlugin,
  IdentifierPlugin,
  StoragePlugin,
} from './types.js';

/**
 * Plugin registry
 * Central registry for all plugins
 */
export class PluginRegistry {
  private plugins: Map<string, Plugin> = new Map();
  private pluginsByType: Map<PluginType, Set<string>> = new Map();

  constructor() {
    // Initialize type maps
    this.pluginsByType.set(PluginType.SOURCE, new Set());
    this.pluginsByType.set(PluginType.IDENTIFIER, new Set());
    this.pluginsByType.set(PluginType.STORAGE, new Set());
  }

  /**
   * Register a plugin
   * @param plugin Plugin instance to register
   */
  register(plugin: Plugin): void {
    const id = plugin.metadata.id;

    if (this.plugins.has(id)) {
      throw new Error(`Plugin with id "${id}" is already registered`);
    }

    this.plugins.set(id, plugin);
    this.pluginsByType.get(plugin.type)?.add(id);

    console.log(`✅ Registered plugin: ${plugin.metadata.name} (${id})`);
  }

  /**
   * Unregister a plugin
   * @param pluginId Plugin ID to unregister
   */
  unregister(pluginId: string): void {
    const plugin = this.plugins.get(pluginId);
    if (!plugin) {
      throw new Error(`Plugin with id "${pluginId}" not found`);
    }

    this.plugins.delete(pluginId);
    this.pluginsByType.get(plugin.type)?.delete(pluginId);

    console.log(`❌ Unregistered plugin: ${plugin.metadata.name} (${pluginId})`);
  }

  /**
   * Get a plugin by ID
   * @param pluginId Plugin ID
   */
  get(pluginId: string): Plugin | undefined {
    return this.plugins.get(pluginId);
  }

  /**
   * Get a source plugin by ID
   * @param pluginId Plugin ID
   */
  getSource(pluginId: string): SourcePlugin | undefined {
    const plugin = this.plugins.get(pluginId);
    return plugin?.type === PluginType.SOURCE ? (plugin as SourcePlugin) : undefined;
  }

  /**
   * Get an identifier plugin by ID
   * @param pluginId Plugin ID
   */
  getIdentifier(pluginId: string): IdentifierPlugin | undefined {
    const plugin = this.plugins.get(pluginId);
    return plugin?.type === PluginType.IDENTIFIER ? (plugin as IdentifierPlugin) : undefined;
  }

  /**
   * Get a storage plugin by ID
   * @param pluginId Plugin ID
   */
  getStorage(pluginId: string): StoragePlugin | undefined {
    const plugin = this.plugins.get(pluginId);
    return plugin?.type === PluginType.STORAGE ? (plugin as StoragePlugin) : undefined;
  }

  /**
   * Get all plugins of a specific type
   * @param type Plugin type
   */
  getByType(type: PluginType): Plugin[] {
    const ids = this.pluginsByType.get(type) || new Set();
    return Array.from(ids)
      .map((id) => this.plugins.get(id))
      .filter((p): p is Plugin => p !== undefined);
  }

  /**
   * Get all source plugins
   */
  getAllSources(): SourcePlugin[] {
    return this.getByType(PluginType.SOURCE) as SourcePlugin[];
  }

  /**
   * Get all identifier plugins
   */
  getAllIdentifiers(): IdentifierPlugin[] {
    return this.getByType(PluginType.IDENTIFIER) as IdentifierPlugin[];
  }

  /**
   * Get all storage plugins
   */
  getAllStorages(): StoragePlugin[] {
    return this.getByType(PluginType.STORAGE) as StoragePlugin[];
  }

  /**
   * Get all registered plugins
   */
  getAll(): Plugin[] {
    return Array.from(this.plugins.values());
  }

  /**
   * Check if a plugin is registered
   * @param pluginId Plugin ID
   */
  has(pluginId: string): boolean {
    return this.plugins.has(pluginId);
  }

  /**
   * Get count of registered plugins
   */
  size(): number {
    return this.plugins.size;
  }

  /**
   * Clear all registered plugins
   */
  clear(): void {
    this.plugins.clear();
    this.pluginsByType.forEach((set) => set.clear());
  }
}

/**
 * Global plugin registry instance
 */
export const pluginRegistry = new PluginRegistry();
