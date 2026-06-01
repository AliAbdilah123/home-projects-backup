import { useState } from 'react';
import ChannelSidebar from './components/ChannelSidebar';
import ConversationList from './components/ConversationList';
import ChatView from './components/ChatView';
import type { Conversation } from './services/api';

export default function App() {
  const [conversation, setConversation] = useState<Conversation>();

  return (
    <div className="app">
      <ChannelSidebar />
      <ConversationList onSelect={setConversation} activeId={conversation?.id} />
      <ChatView conversation={conversation} />
      <style>{css}</style>
    </div>
  );
}

const css = `
  .app {
    font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto;
    background: #0a111c;
    color: #e8edf3;
    height: 100vh;
    display: grid;
    grid-template-columns: 320px 360px 1fr;
    overflow: hidden;
  }
`;
