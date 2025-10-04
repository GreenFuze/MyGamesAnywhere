/**
 * Type declarations for xml-stream
 * Since @types/xml-stream doesn't exist
 */

declare module 'xml-stream' {
  import { Readable } from 'stream';

  class XmlStream {
    constructor(stream: Readable);
    on(event: 'endElement: ' | string, listener: (item: any) => void): this;
    on(event: 'end', listener: () => void): this;
    on(event: 'error', listener: (error: Error) => void): this;
  }

  export = XmlStream;
}
