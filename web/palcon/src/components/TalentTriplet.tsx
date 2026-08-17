import { talentTone } from "../lib/stats";

/** The compact `hp/atk/def` talent readout, with each value tinted by
 * talentTone. Separators inherit the surrounding dim color so the numbers
 * carry the signal. Callers set font/size via className. */
export function TalentTriplet({
  hp,
  attack,
  defense,
  className,
}: {
  hp: number;
  attack: number;
  defense: number;
  className?: string;
}) {
  return (
    <span className={className} title="Talents: HP / Attack / Defense">
      <span style={{ color: talentTone(hp) }}>{hp}</span>
      <span className="opacity-50">/</span>
      <span style={{ color: talentTone(attack) }}>{attack}</span>
      <span className="opacity-50">/</span>
      <span style={{ color: talentTone(defense) }}>{defense}</span>
    </span>
  );
}
