-- Do not manufacture history or silently round money during this breaking migration.
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM events WHERE settled OR archived) THEN
  RAISE EXCEPTION 'PRD migration requires explicit legacy settlement snapshot migration before upgrading';
 END IF;
 IF EXISTS (SELECT 1 FROM item_details WHERE amount_cents % 100 <> 0)
 OR EXISTS (SELECT 1 FROM item_details, jsonb_each_text(custom_shares) kv WHERE (kv.value::numeric) % 100 <> 0) THEN
  RAISE EXCEPTION 'PRD migration cannot round fractional NT dollars; reconcile legacy cent amounts first';
 END IF;
 IF EXISTS (SELECT 1 FROM item_details d JOIN items i ON i.id=d.item_id JOIN event_rules r ON r.event_id=i.event_id AND r.item_tag=d.tag WHERE d.custom_shares<>'{}'::jsonb) THEN
  RAISE EXCEPTION 'PRD migration requires reconciling manual amounts on ruled details';
 END IF;
 IF EXISTS (SELECT 1 FROM events e WHERE (SELECT count(*) FROM event_members m WHERE m.event_id=e.id AND m.role='host')<>1
 OR NOT EXISTS (SELECT 1 FROM event_members m WHERE m.event_id=e.id AND m.role='host' AND m.account_id=e.account_id)) THEN
  RAISE EXCEPTION 'PRD migration requires exactly one host matching the event account';
 END IF;
END $$;
ALTER TABLE item_details RENAME COLUMN amount_cents TO amount;
UPDATE item_details SET amount=amount/100, custom_shares=(SELECT COALESCE(jsonb_object_agg(key,(value::bigint/100)),'{}'::jsonb) FROM jsonb_each_text(custom_shares));
ALTER TABLE item_details ADD COLUMN manual_member_ids BIGINT[];
ALTER TABLE event_members ADD COLUMN split_order BIGINT;
UPDATE event_members SET split_order=id;
ALTER TABLE event_members ALTER COLUMN split_order SET NOT NULL;
CREATE SEQUENCE member_split_order_seq;
SELECT setval('member_split_order_seq', COALESCE((SELECT max(id) FROM event_members),0)+1,false);
ALTER TABLE event_members ALTER COLUMN split_order SET DEFAULT nextval('member_split_order_seq');
ALTER TABLE event_members ADD COLUMN note TEXT NOT NULL DEFAULT '';
ALTER TABLE event_members ADD COLUMN virtual BOOLEAN GENERATED ALWAYS AS (account_id IS NULL AND guest_id IS NULL) STORED;
CREATE UNIQUE INDEX one_host_per_event ON event_members(event_id) WHERE role='host';
ALTER TABLE event_members ADD CONSTRAINT host_requires_account CHECK(role<>'host' OR account_id IS NOT NULL);
ALTER TABLE events ADD COLUMN settlement JSONB;
ALTER TABLE events ADD CONSTRAINT settlement_state CHECK ((NOT settled AND settlement IS NULL AND NOT archived) OR (settled AND settlement IS NOT NULL));
DROP TABLE transfer_payments;

CREATE FUNCTION enforce_event_host() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE eid BIGINT;
BEGIN
 IF TG_TABLE_NAME='events' THEN eid=NEW.id; ELSE
 IF TG_OP='UPDATE' AND NEW.event_id<>OLD.event_id THEN RAISE EXCEPTION 'cannot move child between events' USING ERRCODE='23514'; END IF;
 eid=COALESCE(NEW.event_id,OLD.event_id); END IF;
 IF EXISTS(SELECT 1 FROM events WHERE id=eid) AND NOT EXISTS(
 SELECT 1 FROM events e JOIN event_members m ON m.event_id=e.id
 WHERE e.id=eid AND m.role='host' AND m.account_id=e.account_id) THEN
 RAISE EXCEPTION 'event requires its account-backed host' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER event_host_required AFTER INSERT OR UPDATE ON events DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION enforce_event_host();
CREATE CONSTRAINT TRIGGER member_host_required AFTER INSERT OR UPDATE OR DELETE ON event_members DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION enforce_event_host();

CREATE FUNCTION immutable_settlement() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN
  IF OLD.settled THEN RAISE EXCEPTION 'cannot delete settled history' USING ERRCODE='23514'; END IF;
  RETURN OLD;
 END IF;
 IF OLD.settled AND (NOT NEW.settled OR NEW.settlement IS DISTINCT FROM OLD.settlement OR (to_jsonb(NEW)-'archived') IS DISTINCT FROM (to_jsonb(OLD)-'archived')) THEN
 RAISE EXCEPTION 'settled event is immutable' USING ERRCODE='23514';
 END IF;
 IF OLD.archived AND NOT NEW.archived THEN RAISE EXCEPTION 'cannot unarchive' USING ERRCODE='23514'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER freeze_settlement BEFORE UPDATE OR DELETE ON events FOR EACH ROW EXECUTE FUNCTION immutable_settlement();

CREATE FUNCTION guard_event_child() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE eid BIGINT; closed BOOLEAN;
BEGIN
 IF TG_TABLE_NAME='item_details' THEN
 IF TG_OP='UPDATE' AND NEW.item_id<>OLD.item_id THEN RAISE EXCEPTION 'cannot move detail between cards' USING ERRCODE='23514'; END IF;
 SELECT event_id INTO eid FROM items WHERE id=COALESCE(NEW.item_id,OLD.item_id);
 ELSE
 IF TG_OP='UPDATE' AND NEW.event_id<>OLD.event_id THEN RAISE EXCEPTION 'cannot move child between events' USING ERRCODE='23514'; END IF;
 eid=COALESCE(NEW.event_id,OLD.event_id); END IF;
 SELECT settled OR archived INTO closed FROM events WHERE id=eid FOR UPDATE;
 IF closed THEN RAISE EXCEPTION 'event is read-only' USING ERRCODE='23514'; END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF; RETURN NEW;
END $$;
CREATE TRIGGER freeze_members BEFORE INSERT OR UPDATE OR DELETE ON event_members FOR EACH ROW EXECUTE FUNCTION guard_event_child();
CREATE TRIGGER freeze_items BEFORE INSERT OR UPDATE OR DELETE ON items FOR EACH ROW EXECUTE FUNCTION guard_event_child();
CREATE TRIGGER freeze_details BEFORE INSERT OR UPDATE OR DELETE ON item_details FOR EACH ROW EXECUTE FUNCTION guard_event_child();
CREATE TRIGGER freeze_rules BEFORE INSERT OR UPDATE OR DELETE ON event_rules FOR EACH ROW EXECUTE FUNCTION guard_event_child();
CREATE TRIGGER freeze_item_tags BEFORE INSERT OR UPDATE OR DELETE ON event_item_tags FOR EACH ROW EXECUTE FUNCTION guard_event_child();
CREATE TRIGGER freeze_cond_tags BEFORE INSERT OR UPDATE OR DELETE ON event_cond_tags FOR EACH ROW EXECUTE FUNCTION guard_event_child();
