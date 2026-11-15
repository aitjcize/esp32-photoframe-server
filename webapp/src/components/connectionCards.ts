import type { Component } from 'vue';
import ConnectionCard from './ConnectionCard.vue';
import GoogleConnectionCard from './GoogleConnectionCard.vue';
import TopicConnectionCard from './TopicConnectionCard.vue';
import { TOPIC_SOURCES } from '../stores/connections';

// The card that configures one connection of the given source.
export function cardComponentFor(source: string): Component {
  if (source === 'google_photos') return GoogleConnectionCard;
  if (TOPIC_SOURCES.includes(source)) return TopicConnectionCard;
  return ConnectionCard;
}
